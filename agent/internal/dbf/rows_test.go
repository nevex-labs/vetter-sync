package dbf

import (
	"os"
	"reflect"
	"testing"
	"time"

	"vetter-sync/agent/internal/dbftest"
)

func readAll(t *testing.T, path string) []Row {
	t.Helper()

	var rows []Row

	if err := ReadRows(path, func(row Row) error {
		rows = append(rows, row)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return rows
}

func TestReadRowsTypes(t *testing.T) {
	t.Parallel()

	fields := []dbftest.Field{
		{Name: "CODIGO", Type: 'N', Length: 6},
		{Name: "NOMBRE", Type: 'C', Length: 12},
		{Name: "PESO", Type: 'N', Length: 8, Decimals: 2},
		{Name: "FECHA", Type: 'D', Length: 8},
		{Name: "VIVE", Type: 'L', Length: 1},
		{Name: "CUANDO", Type: 'T', Length: 8},
	}

	at := time.Date(2026, 10, 14, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		record  dbftest.Record
		want    map[string]any
		deleted bool
	}{
		{
			name:   "todos los tipos",
			record: dbftest.Record{Values: []string{"  1524", "Toby", "   12.50", "20261014", "T", dbftest.DateTime(at)}},
			want: map[string]any{
				"CODIGO": int64(1524), "NOMBRE": "Toby", "PESO": 12.5,
				"FECHA": "2026-10-14", "VIVE": int64(1), "CUANDO": "2026-10-14T10:30:00",
			},
		},
		{
			name:   "vacíos son null, nunca cadena vacía",
			record: dbftest.Record{Values: []string{"     7", "", "", "        ", "?", ""}},
			want: map[string]any{
				"CODIGO": int64(7), "NOMBRE": nil, "PESO": nil,
				"FECHA": nil, "VIVE": nil, "CUANDO": nil,
			},
		},
		{
			name:    "borrado se lee igual, con su marca",
			record:  dbftest.Record{Deleted: true, Values: []string{"     9", "Luna\x00\x00", "    0.00", "20250101", "F", ""}},
			deleted: true,
			want: map[string]any{
				"CODIGO": int64(9), "NOMBRE": "Luna", "PESO": 0.0,
				"FECHA": "2025-01-01", "VIVE": int64(0), "CUANDO": nil,
			},
		},
		{
			name:   "desborde y fecha inválida no inventan valores",
			record: dbftest.Record{Values: []string{"******", "x", "********", "20261399", "T", ""}},
			want: map[string]any{
				"CODIGO": nil, "NOMBRE": "x", "PESO": nil,
				"FECHA": nil, "VIVE": int64(1), "CUANDO": nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := dbftest.Write(t, t.TempDir(), "T.DBF", 0x03, fields, []dbftest.Record{tt.record})
			rows := readAll(t, path)

			if len(rows) != 1 {
				t.Fatalf("filas = %d, quería 1", len(rows))
			}

			if rows[0].Deleted != tt.deleted {
				t.Errorf("deleted = %v, quería %v", rows[0].Deleted, tt.deleted)
			}

			if !reflect.DeepEqual(rows[0].Values, tt.want) {
				t.Errorf("valores = %#v\nquería   %#v", rows[0].Values, tt.want)
			}
		})
	}
}

func TestReadRowsCodepage(t *testing.T) {
	t.Parallel()

	fields := []dbftest.Field{{Name: "NOMBRE", Type: 'C', Length: 10}}

	tests := []struct {
		name   string
		driver byte
		raw    string
		want   string
	}{
		{name: "CP1252", driver: 0x03, raw: "\xd1and\xfa", want: "Ñandú"},
		{name: "CP1252 ANSI", driver: 0x57, raw: "P\xe9rez \x80", want: "Pérez €"},
		{name: "CP850", driver: 0x02, raw: "\xa5and\xa3", want: "Ñandú"},
		{name: "CP850 acentos", driver: 0x02, raw: "P\x82rez", want: "Pérez"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := dbftest.Write(t, t.TempDir(), "T.DBF", tt.driver, fields,
				[]dbftest.Record{{Values: []string{tt.raw}}})

			if got := readAll(t, path)[0].Values["NOMBRE"]; got != tt.want {
				t.Errorf("NOMBRE = %q, quería %q", got, tt.want)
			}
		})
	}
}

func TestReadRowsUnknownDriver(t *testing.T) {
	t.Parallel()

	path := dbftest.Write(t, t.TempDir(), "T.DBF", 0x00,
		[]dbftest.Field{{Name: "A", Type: 'C', Length: 1}}, []dbftest.Record{{Values: []string{"a"}}})

	if err := ReadRows(path, func(Row) error { return nil }); err == nil {
		t.Error("un language driver 0x00 tiene que fallar: no se adivina el encoding")
	}
}

// Una copia hecha a mitad de una escritura: el header dice 3 registros y el
// archivo trae 2 completos y medio.
func TestReadRowsTruncated(t *testing.T) {
	t.Parallel()

	fields := []dbftest.Field{{Name: "CODIGO", Type: 'N', Length: 4}}
	data := dbftest.Bytes(0x03, fields, []dbftest.Record{
		{Values: []string{"   1"}}, {Values: []string{"   2"}}, {Values: []string{"   3"}},
	})

	path := t.TempDir() + "/T.DBF"
	if err := os.WriteFile(path, data[:len(data)-4], 0o600); err != nil {
		t.Fatal(err)
	}

	if rows := readAll(t, path); len(rows) != 2 {
		t.Errorf("filas = %d, quería 2 completas", len(rows))
	}
}
