package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vetter-sync/agent/internal/dbftest"
)

func TestCollectAttachments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "estudio.pdf"), []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}

	fields := []dbftest.Field{
		{Name: "ID_ADJ", Type: 'N', Length: 10},
		{Name: "CODIGOPACI", Type: 'C', Length: 9},
		{Name: "FECHA", Type: 'D', Length: 8},
		{Name: "ARCHIVO", Type: 'C', Length: 100},
	}

	today := time.Now().Format("20060102")

	dbftest.Write(t, dir, "ADJUNTOS.DBF", 0x03, fields, []dbftest.Record{
		{Values: []string{"1", "10/1", today, "estudio.pdf"}},
		{Values: []string{"2", "10/1", "20000101", "estudio.pdf"}},
		{Values: []string{"3", "10/1", today, "no-existe.pdf"}},
		{Values: []string{"4", "10/1", today, "programa.exe"}},
		{Deleted: true, Values: []string{"5", "10/1", today, "estudio.pdf"}},
	})

	all, err := collectAttachments(dir, time.Now().AddDate(0, 0, -30), 20<<20)
	if err != nil {
		t.Fatal(err)
	}

	if len(all) != 1 || all[0].ID != "1" || all[0].Name != "estudio.pdf" {
		t.Fatalf("adjuntos = %+v", all)
	}

	if pending := pendingAttachments(all, map[string]string{}); len(pending) != 1 {
		t.Errorf("sin estado tiene que quedar pendiente")
	}

	if pending := pendingAttachments(all, map[string]string{"1": fmt.Sprintf("%d@%d", all[0].Size, all[0].MTime)}); len(pending) != 0 {
		t.Errorf("con el mismo tamaño y mtime no se reenvía: %+v", pending)
	}
}
