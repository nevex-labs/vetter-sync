package report

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeDBF(t *testing.T, dir, name string, phones []string) {
	t.Helper()

	headerBytes := 32 + 2*32 + 1
	recordBytes := 1 + 6 + 20

	head := make([]byte, 32)
	head[0] = 0x30
	head[1], head[2], head[3] = 126, 10, 14
	binary.LittleEndian.PutUint32(head[4:8], uint32(len(phones)))  //nolint:gosec // el fixture entra en uint32
	binary.LittleEndian.PutUint16(head[8:10], uint16(headerBytes)) //nolint:gosec // el fixture entra en uint16
	binary.LittleEndian.PutUint16(head[10:12], uint16(recordBytes))
	head[29] = 0x03

	data := append([]byte{}, head...)

	for _, field := range []struct {
		name   string
		length int
	}{{"CODIGO", 6}, {"TELEFONO", 20}} {
		descriptor := make([]byte, 32)
		copy(descriptor[0:11], field.name)
		descriptor[11] = 'C'
		descriptor[16] = byte(field.length) //nolint:gosec // el fixture entra en un byte
		data = append(data, descriptor...)
	}

	data = append(data, 0x0d)

	for index, phone := range phones {
		data = append(data, ' ')

		code := make([]byte, 6)
		copy(code, []string{"1524", "1525", "1526"}[index])
		data = append(data, code...)

		value := make([]byte, 20)
		copy(value, phone)
		data = append(data, value...)
	}

	data = append(data, 0x1a)

	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "CLIENTES.FPT"), []byte("memo"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDBF(t, dir, "CLIENTES.DBF", []string{"2974112990", "297 411-2990", ""})

	result, err := Build(dir, []StatSpec{{Table: "CLIENTES", Field: "TELEFONO"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 || len(result.Errors) != 0 {
		t.Fatalf("archivos = %d, errores = %+v", len(result.Files), result.Errors)
	}

	file := result.Files[0]

	if file.Memo == nil || file.Memo.Size != 4 {
		t.Errorf("memo = %+v", file.Memo)
	}

	if file.Header.Encoding != "CP1252" {
		t.Errorf("encoding = %s", file.Header.Encoding)
	}

	if file.Records.Header != 3 || !file.Records.Match {
		t.Errorf("registros = %+v", file.Records)
	}

	if len(file.Stats) != 1 || file.Stats[0].Field != "TELEFONO" {
		t.Fatalf("stats = %+v", file.Stats)
	}

	if file.Stats[0].NonEmpty != 2 || file.Stats[0].DigitsOnly != 1 {
		t.Errorf("stats = %+v", file.Stats[0])
	}

	if _, ok := file.Stats[0].Separators["space"]; !ok {
		t.Errorf("separadores = %+v", file.Stats[0].Separators)
	}
}

func TestBuildAvisaStatSinTabla(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDBF(t, dir, "CLIENTES.DBF", []string{"2974112990"})

	result, err := Build(dir, []StatSpec{{Table: "PACIENTES", Field: "CODIGOPACI"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Errors) != 1 || result.Errors[0].File != "PACIENTES" {
		t.Errorf("errores = %+v", result.Errors)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDBF(t, dir, "CLIENTES.DBF", []string{"2974112990"})

	result, err := Build(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "inspector.json")

	if err := Write(path, result); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json inválido: %v", err)
	}

	if parsed["dir"] != result.Dir {
		t.Errorf("dir = %v", parsed["dir"])
	}
}

func TestParseStat(t *testing.T) {
	t.Parallel()

	spec, err := ParseStat("CLIENTES.TELEFONO")
	if err != nil || spec.Table != "CLIENTES" || spec.Field != "TELEFONO" {
		t.Fatalf("spec = %+v, err = %v", spec, err)
	}

	for _, bad := range []string{"", "CLIENTES", "A.B.C", ".CAMPO", "TABLA."} {
		if _, err := ParseStat(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}
