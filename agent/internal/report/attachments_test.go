package report

import (
	"os"
	"path/filepath"
	"testing"

	"vetter-sync/agent/internal/dbftest"
)

func TestScanAttachments(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	mirror := t.TempDir()

	write := func(dir, name string, size int) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(source, "estudio.pdf", 2<<20)
	write(mirror, "solo-onedrive.jpg", 3<<20)

	fields := []dbftest.Field{
		{Name: "ID_ADJ", Type: 'N', Length: 10},
		{Name: "ARCHIVO", Type: 'C', Length: 100},
	}

	dbftest.Write(t, source, "ADJUNTOS.DBF", 0x03, fields, []dbftest.Record{
		{Values: []string{"1", "estudio.pdf"}},
		{Values: []string{"2", "solo-onedrive.jpg"}},
		{Values: []string{"3", "no-existe.pdf"}},
		{Values: []string{"4", ""}},
		{Deleted: true, Values: []string{"5", "estudio.pdf"}},
	})

	dbftest.Write(t, source, "FOTOS.DBF", 0x03, fields, []dbftest.Record{
		{Values: []string{"1", "estudio.pdf"}},
	})

	result, err := ScanAttachments(source, mirror)
	if err != nil {
		t.Fatal(err)
	}

	adj := result.Tables["ADJUNTOS"]

	if adj.Rows != 5 || adj.Deleted != 1 || adj.WithPath != 3 || adj.WithoutPath != 1 {
		t.Errorf("conteos = %+v", adj)
	}

	if adj.InSource != 1 || adj.InMirrorOnly != 1 || adj.Missing != 1 || adj.InBoth != 0 {
		t.Errorf("existencia = %+v", adj)
	}

	if adj.Extensions["pdf"] != 2 || adj.Extensions["jpg"] != 1 {
		t.Errorf("extensiones = %v", adj.Extensions)
	}

	if adj.Buckets["1_5MB"] != 2 {
		t.Errorf("tamaños = %v", adj.Buckets)
	}

	if result.Tables["FOTOS"].InSource != 1 {
		t.Errorf("FOTOS = %+v", result.Tables["FOTOS"])
	}
}
