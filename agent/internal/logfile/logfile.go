// Package logfile abre el log del día (agent-AAAA-MM-DD.log) y borra los de
// más de 7 días. Una línea JSON por evento; nunca PII.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Retention es cuánto se guardan los logs.
const Retention = 7 * 24 * time.Hour

// Open abre (o crea) el log del día en dir y limpia los viejos.
func Open(dir string, now time.Time) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("crear %s: %w", dir, err)
	}

	prune(dir, now)

	path := filepath.Join(dir, "agent-"+now.Format(time.DateOnly)+".log")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", path, err)
	}

	return file, nil
}

// prune borra agent-*.log con fecha en el nombre anterior a la retención. Es
// mejor esfuerzo: un log que no se puede borrar no corta la corrida.
func prune(dir string, now time.Time) {
	paths, err := filepath.Glob(filepath.Join(dir, "agent-*.log"))
	if err != nil {
		return
	}

	limit := now.Add(-Retention)

	for _, path := range paths {
		day := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "agent-"), ".log")

		stamp, err := time.ParseInLocation(time.DateOnly, day, now.Location())
		if err != nil || !stamp.Before(limit) {
			continue
		}

		_ = os.Remove(path)
	}
}
