package syncer

import (
	"strings"
	"testing"

	"vetter-sync/agent/internal/dbftest"
)

// Las dos copias de la clínica divergen: el aviso tiene que decir qué tabla y
// cuántos registros tiene cada lado, sin filas ni valores.
func TestMirrorDifferences(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	mirror := t.TempDir()

	fields := []dbftest.Field{{Name: "CODIGO", Type: 'N', Length: 6}}

	counts := map[string]int{"CLIENTES": 2, "PACIENTES": 3, "VACUNAS": 1, "VISITAS": 4, "ADJUNTOS": 2}

	for table, n := range counts {
		name := strings.ToUpper(table) + ".dbf"
		records := make([]dbftest.Record, n)

		for i := range records {
			records[i] = dbftest.Record{Values: []string{"1"}}
		}

		dbftest.Write(t, source, name, 0x03, fields, records)

		// OneDrive tiene un paciente más y una visita menos.
		m := n

		switch table {
		case "PACIENTES":
			m++
		case "VISITAS":
			m--
		}

		more := make([]dbftest.Record, m)

		for i := range more {
			more[i] = dbftest.Record{Values: []string{"1"}}
		}

		dbftest.Write(t, mirror, name, 0x03, fields, more)
	}

	got := mirrorDifferences(source, mirror)

	if len(got) != 2 {
		t.Fatalf("diferencias = %v, quería 2", got)
	}

	if !strings.Contains(got[0], "PACIENTES: UNC 3, OneDrive 4") {
		t.Errorf("primera diferencia = %q", got[0])
	}

	if !strings.Contains(got[1], "VISITAS: UNC 4, OneDrive 3") {
		t.Errorf("segunda diferencia = %q", got[1])
	}
}

func TestMirrorDifferencesIguales(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	mirror := t.TempDir()

	fields := []dbftest.Field{{Name: "CODIGO", Type: 'N', Length: 6}}
	records := []dbftest.Record{{Values: []string{"1"}}}

	for _, table := range Tables() {
		name := strings.ToUpper(table) + ".dbf"

		dbftest.Write(t, source, name, 0x03, fields, records)
		dbftest.Write(t, mirror, name, 0x03, fields, records)
	}

	if got := mirrorDifferences(source, mirror); len(got) != 0 {
		t.Fatalf("sin diferencias = %v", got)
	}
}
