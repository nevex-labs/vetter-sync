// Package dbftest arma DBF sintéticos para los tests. Nunca datos de la
// clínica: los valores los escribe cada test.
package dbftest

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Field describe una columna del DBF sintético.
type Field struct {
	Name     string
	Type     byte
	Length   int
	Decimals int
}

// Record es una fila: un valor crudo por campo (se rellena con espacios a la
// derecha). Para campos binarios (T, I) el string lleva los bytes tal cual.
type Record struct {
	Deleted bool
	Values  []string
}

// Write crea dir/name con header Visual FoxPro (0x30, con los 263 bytes del
// backlink) y devuelve la ruta.
func Write(t *testing.T, dir, name string, driver byte, fields []Field, records []Record) string {
	t.Helper()

	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, Bytes(driver, fields, records), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// Bytes arma el contenido del DBF.
func Bytes(driver byte, fields []Field, records []Record) []byte {
	headerBytes := 32 + len(fields)*32 + 1 + 263
	recordBytes := 1

	for _, field := range fields {
		recordBytes += field.Length
	}

	head := make([]byte, 32)
	head[0] = 0x30
	head[1], head[2], head[3] = 126, 10, 14
	binary.LittleEndian.PutUint32(head[4:8], uint32(len(records)))  //nolint:gosec // fixture chico
	binary.LittleEndian.PutUint16(head[8:10], uint16(headerBytes))  //nolint:gosec // fixture chico
	binary.LittleEndian.PutUint16(head[10:12], uint16(recordBytes)) //nolint:gosec // fixture chico
	head[29] = driver

	data := append([]byte{}, head...)

	for _, field := range fields {
		descriptor := make([]byte, 32)
		copy(descriptor[0:11], field.Name)
		descriptor[11] = field.Type
		descriptor[16] = byte(field.Length)   //nolint:gosec // fixture chico
		descriptor[17] = byte(field.Decimals) //nolint:gosec // fixture chico
		data = append(data, descriptor...)
	}

	data = append(data, 0x0d)
	data = append(data, make([]byte, 263)...)

	for _, record := range records {
		flag := byte(' ')
		if record.Deleted {
			flag = '*'
		}

		data = append(data, flag)

		for i, field := range fields {
			value := make([]byte, field.Length)
			for j := range value {
				value[j] = ' '
			}

			if i < len(record.Values) {
				copy(value, record.Values[i])
			}

			data = append(data, value...)
		}
	}

	return append(data, 0x1a)
}

// DateTime codifica un T de Visual FoxPro: día juliano y milisegundos.
func DateTime(at time.Time) string {
	raw := make([]byte, 8)
	day := at.Unix()/86400 + 2440588
	millis := (at.Unix() % 86400) * 1000

	binary.LittleEndian.PutUint32(raw[0:4], uint32(day))    //nolint:gosec // fixture chico
	binary.LittleEndian.PutUint32(raw[4:8], uint32(millis)) //nolint:gosec // fixture chico

	return string(raw)
}
