package logfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenPrunesOldLogs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	files := map[string]bool{
		"agent-2026-11-01.log": false, // viejo: se borra
		"agent-2026-11-10.log": false, // 8 días: se borra
		"agent-2026-11-12.log": true,
		"agent-roto.log":       true, // no es una fecha: no se toca
		"otro.txt":             true,
	}

	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	file, err := Open(dir, now)
	if err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if filepath.Base(file.Name()) != "agent-2026-11-18.log" {
		t.Errorf("log = %s", file.Name())
	}

	for name, keep := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if exists := err == nil; exists != keep {
			t.Errorf("%s: existe=%v, quería %v", name, exists, keep)
		}
	}
}
