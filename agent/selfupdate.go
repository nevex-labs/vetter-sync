package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vetter-sync/agent/internal/syncer"
	"vetter-sync/agent/internal/update"
)

// checkInterval es cada cuánto se consulta el release. La tarea corre cada 15
// minutos; el manifiesto se mira una vez por día.
const checkInterval = 24 * time.Hour

// maxAttempts es cuántas corridas seguidas puede fallar una versión nueva antes
// de volver a la anterior.
const maxAttempts = 3

// selfUpdate consulta el release firmado y, si hay una versión mayor, deja el
// binario nuevo para la próxima corrida. Nunca corta el sync: si algo falla,
// se loguea y se sigue con lo que hay.
func selfUpdate(ctx context.Context, dir string, logger *slog.Logger) {
	if updatePublicKey == "" {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		logger.Warn("update: no se pudo ubicar el ejecutable", "err", err)

		return
	}

	checkedPath := filepath.Join(dir, "update.checked")

	// Si la versión nueva falló tres corridas seguidas, vuelve la anterior.
	if readCounter(filepath.Join(dir, "update.attempts")) >= maxAttempts {
		if err := update.Restore(exe); err != nil {
			logger.Warn("update: no se pudo restaurar la versión anterior", "err", err)
		} else {
			logger.Warn("update: la versión nueva falló tres veces, restaurada la anterior")
		}

		writeCounter(filepath.Join(dir, "update.attempts"), 0)

		return
	}

	if time.Since(update.CheckedAt(checkedPath)) < checkInterval {
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	release, err := latestRelease(ctx, client)
	if err != nil {
		logger.Warn("update: no se pudo leer el release", "err", err)

		return
	}

	if !update.Newer(release.Version, version) {
		_ = update.MarkChecked(checkedPath, time.Now())

		return
	}

	if release.MinContract > 0 && release.MinContract > syncer.Contract {
		logger.Warn("update: el release pide un contrato más nuevo", "release", release.Version,
			"min_contract", release.MinContract, "agente", syncer.Contract)

		return
	}

	if err := update.Download(ctx, client, release, exe, exe+".new"); err != nil {
		logger.Warn("update: no se pudo bajar la versión", "err", err)

		return
	}

	if err := update.Rotate(exe); err != nil {
		logger.Warn("update: no se pudo instalar la versión", "err", err)

		return
	}

	_ = update.MarkChecked(checkedPath, time.Now())
	logger.Info("update: instalada, la próxima corrida la usa", "version", release.Version)
}

// latestRelease baja y valida el manifiesto del último release.
func latestRelease(ctx context.Context, client *http.Client) (update.Release, error) {
	manifest, signature, err := update.Fetch(ctx, client)
	if err != nil {
		return update.Release{}, fmt.Errorf("consultar el release: %w", err)
	}

	release, err := update.Decode(manifest, signature, updatePublicKey)
	if err != nil {
		return update.Release{}, fmt.Errorf("manifiesto rechazado: %w", err)
	}

	return release, nil
}

func readCounter(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	return n
}

func writeCounter(path string, n int) {
	_ = os.WriteFile(path, []byte(strconv.Itoa(n)), 0o600)
}

// countAttempt suma una corrida. Se reinicia cuando la corrida termina bien.
func countAttempt(dir string) int {
	path := filepath.Join(dir, "update.attempts")
	next := readCounter(path) + 1

	writeCounter(path, next)

	return next
}
