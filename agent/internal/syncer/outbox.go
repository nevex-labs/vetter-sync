package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (r *Runner) outboxDir() string {
	return filepath.Join(r.Config.CopyDir, "outbox")
}

// toOutbox guarda en disco los lotes que no se pudieron mandar por 500 o red,
// para mandarlos primero en la corrida siguiente.
func (r *Runner) toOutbox(rows []Row) error {
	stamp := time.Now().UTC().Format("20060102T150405.000000000")

	for start, part := 0, 0; start < len(rows); start, part = start+MaxBatchRows, part+1 {
		batch := Batch{Rows: rows[start:min(start+MaxBatchRows, len(rows))]}

		data, err := EncodeBatch(batch)
		if err != nil {
			return err
		}

		path := filepath.Join(r.outboxDir(), fmt.Sprintf("%s-%03d.json", stamp, part))

		if err := writeAtomic(path, data); err != nil {
			return fmt.Errorf("outbox: %w", err)
		}
	}

	r.Logger.Warn("filas al outbox", "filas", len(rows))

	return nil
}

// flushOutbox manda lo pendiente de corridas anteriores, en orden. Devuelve
// true si el servidor sigue caído (lo que queda se deja para la próxima).
func (r *Runner) flushOutbox(ctx context.Context, snap *Snapshot) (bool, error) {
	paths, err := filepath.Glob(filepath.Join(r.outboxDir(), "*.json"))
	if err != nil {
		return false, fmt.Errorf("outbox: %w", err)
	}

	sort.Strings(paths)

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("outbox %s: %w", path, err)
		}

		var batch Batch

		if err := json.Unmarshal(data, &batch); err != nil {
			r.Logger.Error("outbox ilegible, se descarta", "archivo", filepath.Base(path), "err", err)
			_ = os.Remove(path)

			continue
		}

		_, err = r.sendSplitting(ctx, snap, batch.Rows)

		switch {
		case err == nil:
		case errors.Is(err, ErrServer):
			return true, nil
		default:
			return false, err
		}

		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("outbox %s: %w", path, err)
		}
	}

	if len(paths) > 0 {
		r.Logger.Info("outbox vaciado", "archivos", len(paths))
	}

	return false, nil
}

// OutboxSize cuenta los lotes pendientes en disco.
func OutboxSize(copyDir string) int {
	paths, err := filepath.Glob(filepath.Join(copyDir, "outbox", "*.json"))
	if err != nil {
		return 0
	}

	return len(paths)
}
