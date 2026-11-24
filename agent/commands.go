package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"vetter-sync/agent/internal/logfile"
	"vetter-sync/agent/internal/secret"
	"vetter-sync/agent/internal/syncer"
)

func defaultConfigPath() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = "."
	}

	return filepath.Join(base, "vetter-agent", "config.json")
}

func newClient(cfg syncer.Config, token string) *syncer.Client {
	return &syncer.Client{
		Endpoint:  cfg.Endpoint,
		Secret:    token,
		UserAgent: fmt.Sprintf("vetter-agent/%s (%s)", version, cfg.InstallID),
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		Backoff:   syncer.DefaultBackoff(),
	}
}

// pair pide el secreto sin eco, lo valida con un lote vacío y lo guarda con
// DPAPI. Se corre una vez, con la misma cuenta que la tarea programada.
func pair(args []string) int {
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	configPath := flags.String("config", defaultConfigPath(), "config.json de la PC")
	fromClipboard := flags.Bool("clipboard", false, "leer el secreto del portapapeles (y vaciarlo) en vez del prompt")
	window := flags.Bool("gui", false, "pedir el secreto en una ventana (se pega normal) y mostrar el resultado")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	report := func(text string, failed bool) int {
		if failed {
			fmt.Fprintln(os.Stderr, text)
		} else {
			fmt.Println(text)
		}

		if *window {
			secret.Notify(text, failed)
		}

		if failed {
			return exitError
		}

		return exitOK
	}

	cfg, err := syncer.LoadConfig(*configPath)
	if err != nil {
		return report(err.Error(), true)
	}

	token, err := readToken(*window, *fromClipboard)
	if err != nil {
		return report(err.Error(), true)
	}

	if err := secret.Validate(token); err != nil {
		return report(err.Error(), true)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	text, failed := pairWith(ctx, cfg, token)

	return report(text, failed)
}

func readToken(window, fromClipboard bool) (string, error) {
	var (
		token  string
		err    error
		source string
	)

	switch {
	case window:
		source = "ventana"
		token, err = secret.PromptWindow()
	case fromClipboard:
		source = "portapapeles"
		token, err = secret.ReadClipboard()
		if err == nil {
			fmt.Fprintf(os.Stderr, "Secreto leído del portapapeles (%d caracteres); el portapapeles quedó vacío.\n", len(token))
		}
	default:
		source = "consola"
		token, err = secret.Prompt("Secreto de instalación (vgs_…, se ve un * por carácter): ", os.Stdin, os.Stderr)
	}

	if err != nil {
		return "", fmt.Errorf("secreto desde %s: %w", source, err)
	}

	return token, nil
}

// pairWith valida el secreto con un lote vacío y, si el Worker responde 200,
// lo guarda. Devuelve el texto para el usuario y si falló.
func pairWith(ctx context.Context, cfg syncer.Config, token string) (string, bool) {
	client := newClient(cfg, token)
	client.Backoff = nil

	if _, err := client.Send(ctx, syncer.Batch{}); err != nil {
		if errors.Is(err, syncer.ErrUnauthorized) {
			return "401: el servidor rechazó el secreto (inválido o revocado). No se guardó nada.", true
		}

		return fmt.Sprintf("No se pudo validar el secreto: %v. No se guardó nada.", err), true
	}

	candidates := secret.Candidates()

	path, err := secret.Save(ctx, token, candidates)
	if path == "" {
		return err.Error(), true
	}

	text := "Pareo OK. Secreto cifrado (DPAPI LocalMachine) en " + path

	if err != nil {
		text += "\nAviso: " + err.Error()
	}

	if len(candidates) > 0 && path != candidates[0] {
		text += fmt.Sprintf("\nSin permiso en %s: se usó %s. La tarea tiene que correr con esta misma cuenta.",
			filepath.Dir(candidates[0]), filepath.Dir(path))
	}

	return text, false
}

func syncCommand(args []string) int {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	configPath := flags.String("config", "", "config.json de la PC")
	dryRun := flags.Bool("dry-run", false, "copia y cuenta por tabla sin tocar la red ni el snapshot")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "falta --config")
		return exitUsage
	}

	cfg, err := syncer.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}

	logger, closeLog := openLogger(cfg.LogDir)
	defer closeLog()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *dryRun {
		return dryRunCommand(ctx, cfg, logger)
	}

	return realSync(ctx, cfg, logger)
}

func openLogger(dir string) (*slog.Logger, func()) {
	var out io.Writer = os.Stderr

	closeFn := func() {}

	if dir != "" {
		file, err := logfile.Open(dir, time.Now())
		if err == nil {
			out = io.MultiWriter(file, os.Stderr)
			closeFn = func() { _ = file.Close() }
		} else {
			fmt.Fprintf(os.Stderr, "sin log en disco: %v\n", err)
		}
	}

	return slog.New(slog.NewJSONHandler(out, nil)), closeFn
}

func dryRunCommand(ctx context.Context, cfg syncer.Config, logger *slog.Logger) int {
	runner := syncer.Runner{Config: cfg, Version: version, Logger: logger, DryRun: true}

	summary, err := runner.Run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}

	fmt.Printf("\nDry-run — %s (sin red, snapshot sin tocar)\n", cfg.ZPath)
	fmt.Printf("%-10s %8s %8s %10s %10s %7s %13s\n",
		"tabla", "filas", "borradas", "pendientes", "ids dup.", "sin id", "desaparecidas")

	for _, table := range summary.Tables {
		if table.Error != "" {
			fmt.Printf("%-10s error: %s\n", table.Table, table.Error)
			continue
		}

		fmt.Printf("%-10s %8d %8d %10d %10d %7d %13d\n", table.Table, table.Rows, table.Deleted,
			table.Pending, table.Duplicates, table.EmptyKey, table.Missing)
	}

	fmt.Printf("\nOutbox: %d lotes pendientes\n", syncer.OutboxSize(cfg.CopyDir))

	for _, warning := range summary.Warnings {
		fmt.Printf("Aviso: %s\n", warning)
	}

	if len(summary.Errors) > 0 {
		return exitError
	}

	return exitOK
}

// realSync hace el chequeo previo (secreto, UNC, contrato) y la corrida. Si
// algo falla, lo loguea, intenta mandar agent_error y sale con 1.
func realSync(ctx context.Context, cfg syncer.Config, logger *slog.Logger) int {
	dir := filepath.Dir(cfg.Snapshot)

	// Cada corrida cuenta para el rollback; se borra cuando termina bien.
	countAttempt(dir)
	selfUpdate(ctx, dir, logger)

	token, path, err := secret.Load(secret.Candidates())
	if err != nil {
		logger.Error("secreto ilegible", "err", err)
		return exitError
	}

	logger.Info("corrida", "version", version, "install_id", cfg.InstallID, "secreto", path)

	client := newClient(cfg, token)

	if _, err := os.Stat(cfg.ZPath); err != nil {
		logger.Error("share inalcanzable", "err", err)
		syncer.ReportFailure(ctx, client, logger, "share inalcanzable")

		return exitError
	}

	contract, err := client.Ping(ctx)
	if err != nil {
		logger.Error("endpoint inalcanzable", "err", err)
		return exitError
	}

	if contract != syncer.Contract {
		logger.Error("contrato distinto: no se manda", "servidor", contract, "agente", syncer.Contract)
		syncer.ReportFailure(ctx, client, logger,
			fmt.Sprintf("contrato %d en el servidor, el agente entiende %d", contract, syncer.Contract))

		return exitError
	}

	runner := syncer.Runner{Config: cfg, Version: version, Client: client, Logger: logger}

	summary, err := runner.Run(ctx)
	if err != nil {
		logger.Error("corrida cortada", "err", err)
		return exitError
	}

	for _, table := range summary.Tables {
		logger.Info("resumen", "tabla", table.Table, "filas", table.Rows, "pendientes", table.Pending,
			"confirmadas", table.Confirmed, "sin_cambios", table.Unchanged, "error", table.Error)
	}

	for _, warning := range summary.Warnings {
		logger.Warn("aviso", "detalle", warning)
	}

	// Llegar acá con la corrida hecha confirma que la versión instalada corre.
	writeCounter(filepath.Join(dir, "update.attempts"), 0)

	if len(summary.Errors) > 0 {
		return exitError
	}

	return exitOK
}
