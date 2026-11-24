package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vetter-sync/agent/internal/dbf"
	"vetter-sync/agent/internal/secret"
	"vetter-sync/agent/internal/syncer"
)

const (
	filesPerRun = 20
	maxFileMB   = 20
	filesDays   = 90
)

var allowedFiles = map[string]bool{
	"pdf": true, "jpg": true, "jpeg": true, "png": true, "webp": true,
	"mp4": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
}

type attachment struct {
	ID    string
	Pet   string
	Date  string
	Name  string
	Path  string
	Size  int64
	MTime int64
}

// filesCommand sube a R2 los adjuntos recientes que todavía no están en el
// servidor. Es idempotente: el estado local evita reenviar lo ya subido.
func filesCommand(args []string) int {
	flags := flag.NewFlagSet("files", flag.ContinueOnError)

	configPath := flags.String("config", defaultConfigPath(), "config.json de la PC")
	dir := flags.String("dir", "", "carpeta con los DBF (UNC)")
	days := flags.Int("days", filesDays, "antigüedad máxima del estudio, en días")
	maxMB := flags.Int("max-mb", maxFileMB, "tamaño máximo por archivo, en MB")
	limit := flags.Int("limit", filesPerRun, "archivos por corrida")
	dry := flags.Bool("dry-run", false, "listar sin subir")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "falta --dir")
		return exitUsage
	}

	cfg, err := syncer.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	statePath := filepath.Join(filepath.Dir(cfg.Snapshot), "files.json")
	state := loadFileState(statePath)

	all, err := collectAttachments(*dir, time.Now().AddDate(0, 0, -*days), int64(*maxMB)<<20)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	pending := pendingAttachments(all, state)

	fmt.Printf("Adjuntos recientes: %d, para subir: %d\n", len(all), len(pending))

	if *dry {
		for _, item := range pending {
			fmt.Printf("  %s  %s  %d KB\n", item.Date, item.Name, item.Size/1024)
		}

		return exitOK
	}

	if len(pending) > *limit {
		pending = pending[:*limit]
	}

	token, _, err := secret.Load(secret.Candidates())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	uploaded := 0

	for _, item := range pending {
		if err := uploadAttachment(ctx, cfg, token, item); err != nil {
			if errors.Is(err, errUnauthorized) {
				fmt.Fprintln(os.Stderr, err)

				return exitError
			}

			fmt.Fprintf(os.Stderr, "  %s: %v\n", item.Name, err)
			continue
		}

		state[item.ID] = fmt.Sprintf("%d@%d", item.Size, item.MTime)
		saveFileState(statePath, state)
		uploaded++
	}

	fmt.Printf("Subidos: %d\n", uploaded)

	return exitOK
}

var errUnauthorized = errors.New("401: el servidor rechazó el secreto")

func collectAttachments(dir string, since time.Time, maxBytes int64) ([]attachment, error) {
	path, err := findTable(dir, "ADJUNTOS")
	if err != nil {
		return nil, err
	}

	var out []attachment

	err = dbf.ReadRows(path, func(row dbf.Row) error {
		if row.Deleted {
			return nil
		}

		raw := strings.TrimSpace(fmt.Sprint(row.Values["ARCHIVO"]))
		if raw == "" || raw == "<nil>" {
			return nil
		}

		if !allowedFiles[strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.ReplaceAll(raw, "\\", "/")), "."))] {
			return nil
		}

		date := strings.TrimSpace(fmt.Sprint(row.Values["FECHA"]))
		if len(date) != 10 {
			return nil
		}

		when, err := time.Parse("2006-01-02", date)
		if err != nil || when.Before(since) {
			return nil
		}

		resolved, size, mtime, err := resolveFile(dir, raw)
		if err != nil || size > maxBytes || size == 0 {
			return nil
		}

		out = append(out, attachment{
			ID:    strings.TrimSpace(fmt.Sprint(row.Values["ID_ADJ"])),
			Pet:   strings.TrimSpace(fmt.Sprint(row.Values["CODIGOPACI"])),
			Date:  date,
			Name:  filepath.Base(strings.ReplaceAll(raw, "\\", "/")),
			Path:  resolved,
			Size:  size,
			MTime: mtime,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("leer adjuntos: %w", err)
	}

	return out, nil
}

func resolveFile(root, raw string) (string, int64, int64, error) {
	relative := strings.ReplaceAll(raw, "\\", string(filepath.Separator))
	relative = strings.TrimPrefix(relative, string(filepath.Separator))

	candidates := []string{
		filepath.Join(root, relative),
		filepath.Join(filepath.Dir(root), relative),
	}

	var last = os.ErrNotExist

	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Clean(candidate))
		if err == nil && !info.IsDir() {
			return candidate, info.Size(), info.ModTime().Unix(), nil
		}

		last = err
	}

	return "", 0, 0, last
}

func pendingAttachments(all []attachment, state map[string]string) []attachment {
	var out []attachment

	for _, item := range all {
		if state[item.ID] == fmt.Sprintf("%d@%d", item.Size, item.MTime) {
			continue
		}

		out = append(out, item)
	}

	return out
}

func uploadAttachment(ctx context.Context, cfg syncer.Config, token string, item attachment) error {
	file, err := os.Open(item.Path)
	if err != nil {
		return fmt.Errorf("abrir: %w", err)
	}
	defer func() { _ = file.Close() }()

	endpoint := strings.TrimSuffix(cfg.Endpoint, "/vetter") + "/vetter-file"

	query := url.Values{}
	query.Set("table", "ADJUNTOS")
	query.Set("id", item.ID)
	query.Set("pet", item.Pet)
	query.Set("date", item.Date)
	query.Set("name", item.Name)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?"+query.Encode(), file)
	if err != nil {
		return fmt.Errorf("armar pedido: %w", err)
	}

	req.ContentLength = item.Size
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "vetter-agent/"+version+" ("+cfg.InstallID+")")

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("subir: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))

		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func loadFileState(path string) map[string]string {
	state := map[string]string{}

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}

	return state
}

func saveFileState(path string, state map[string]string) {
	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	_ = os.WriteFile(path, data, 0o600)
}

func findTable(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("listar %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name+".dbf") {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no está %s.dbf en %s", name, dir)
}
