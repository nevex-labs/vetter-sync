package dbf

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

type testField struct {
	name   string
	kind   byte
	length int
}

func writeTestDBF(t *testing.T, records [][]string, deleted []bool) string {
	t.Helper()

	fields := []testField{
		{name: "CODIGO", kind: 'C', length: 6},
		{name: "TELEFONO", kind: 'C', length: 20},
	}

	headerBytes := 32 + len(fields)*32 + 1
	recordBytes := 1

	for _, field := range fields {
		recordBytes += field.length
	}

	head := make([]byte, 32)
	head[0] = 0x30
	head[1], head[2], head[3] = 126, 10, 14
	binary.LittleEndian.PutUint32(head[4:8], uint32(len(records)))
	binary.LittleEndian.PutUint16(head[8:10], uint16(headerBytes))
	binary.LittleEndian.PutUint16(head[10:12], uint16(recordBytes))
	head[29] = 0x03

	data := append([]byte{}, head...)

	for _, field := range fields {
		descriptor := make([]byte, 32)
		copy(descriptor[0:11], field.name)
		descriptor[11] = field.kind
		descriptor[16] = byte(field.length)
		data = append(data, descriptor...)
	}

	data = append(data, 0x0d)

	for index, record := range records {
		data = append(data, ' ')

		if deleted[index] {
			data[len(data)-1] = '*'
		}

		for i, field := range fields {
			value := make([]byte, field.length)
			copy(value, record[i])
			data = append(data, value...)
		}
	}

	data = append(data, 0x1a)

	path := filepath.Join(t.TempDir(), "CLIENTES.DBF")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestOpen(t *testing.T) {
	path := writeTestDBF(t, [][]string{
		{"1524", "2974112990"},
		{"1525", "297 411-2990"},
		{"1526", ""},
	}, []bool{false, true, false})

	table, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if table.Header.Version != 0x30 {
		t.Errorf("versión = %#x, quería 0x30", table.Header.Version)
	}

	if table.Header.UpdatedAt != "2026-10-14" {
		t.Errorf("fecha = %s", table.Header.UpdatedAt)
	}

	if table.Header.Records != 3 || table.Scanned != 3 {
		t.Errorf("registros = %d, escaneados = %d", table.Header.Records, table.Scanned)
	}

	if table.Deleted != 1 {
		t.Errorf("borrados = %d, quería 1", table.Deleted)
	}

	if table.Header.LanguageDriver != 0x03 {
		t.Errorf("language driver = %#x", table.Header.LanguageDriver)
	}

	if len(table.Fields) != 2 || table.Fields[0].Name != "CODIGO" || table.Fields[1].Length != 20 {
		t.Errorf("campos = %+v", table.Fields)
	}

	if table.Header.Records != table.Computed {
		t.Errorf("header %d vs computado %d", table.Header.Records, table.Computed)
	}
}

func TestInspect(t *testing.T) {
	path := writeTestDBF(t, [][]string{
		{"1524", "2974112990"},
		{"1525", "297 411-2990"},
		{"1526", "297 411-2990"},
		{"1527", ""},
		{"1528", "297,4112990"},
	}, []bool{false, true, false, false, false})

	table, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	stats, err := table.Inspect("telefono")
	if err != nil {
		t.Fatal(err)
	}

	// El borrado no cuenta.
	if stats.Scanned != 4 {
		t.Errorf("escaneados = %d, quería 4", stats.Scanned)
	}

	if stats.NonEmpty != 3 {
		t.Errorf("con datos = %d, quería 3", stats.NonEmpty)
	}

	if stats.DigitsOnly != 1 {
		t.Errorf("solo dígitos = %d, quería 1", stats.DigitsOnly)
	}

	if stats.HasSpace != 1 || stats.HasDash != 1 || stats.HasComma != 1 {
		t.Errorf("separadores = %+v", stats)
	}

	if stats.Lengths[10] != 1 || stats.Lengths[11] != 1 {
		t.Errorf("largos = %+v", stats.Lengths)
	}

	if _, err := table.Inspect("NO_EXISTE"); err == nil {
		t.Error("un campo inexistente tiene que fallar")
	}
}
