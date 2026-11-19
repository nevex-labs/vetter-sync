package dbf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Row es un registro decodificado: nombre de campo → valor tipado según la
// regla v1 del contrato (string, int64, float64, "AAAA-MM-DD", 0/1 o nil).
type Row struct {
	Deleted bool
	Values  map[string]any
}

// Días julianos de referencia para los campos T de Visual FoxPro.
const (
	julianYearOne   = 1721426 // 0001-01-01
	julianUnixEpoch = 2440588 // 1970-01-01
	julianYear9999  = 5373484 // 9999-12-31
)

// ReadRows recorre todos los registros, borrados incluidos, y llama a fn con
// cada uno. Se usa sobre la copia local, nunca sobre el original.
//
// Si el header dice más registros de los que entran en el archivo (copia a
// mitad de una escritura), lee solo los completos.
func ReadRows(path string, fn func(Row) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	head, err := readHeader(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	fields, err := readFields(file, head.HeaderBytes)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	codepage, err := CodepageFor(head.LanguageDriver)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	width := 1
	for _, field := range fields {
		width += field.Length
	}

	if width > head.RecordBytes {
		return fmt.Errorf("%s: los campos suman %d bytes y el registro tiene %d", path, width, head.RecordBytes)
	}

	if _, err := file.Seek(int64(head.HeaderBytes), io.SeekStart); err != nil {
		return fmt.Errorf("seek %s: %w", path, err)
	}

	total := min(head.Records, computedRecords(info.Size(), head))
	reader := bufio.NewReaderSize(file, 64*1024)
	record := make([]byte, head.RecordBytes)

	for i := 0; i < total; i++ {
		if _, err := io.ReadFull(reader, record); err != nil {
			return fmt.Errorf("%s registro %d: %w", path, i, err)
		}

		row := Row{Deleted: record[0] == '*', Values: make(map[string]any, len(fields))}
		offset := 1

		for _, field := range fields {
			raw := record[offset : offset+field.Length]
			offset += field.Length

			// _NullFlags y otras columnas de sistema de Visual FoxPro.
			if field.Type == '0' {
				continue
			}

			row.Values[field.Name] = decodeValue(field, raw, &codepage)
		}

		if err := fn(row); err != nil {
			return err
		}
	}

	return nil
}

func decodeValue(field Field, raw []byte, codepage *Codepage) any {
	switch field.Type {
	case 'N', 'F':
		return decodeNumber(strings.TrimSpace(string(raw)), field.Decimals)
	case 'D':
		return decodeDate(string(raw))
	case 'T':
		return decodeDateTime(raw)
	case 'L':
		return decodeLogical(raw)
	case 'I':
		if len(raw) != 4 {
			return nil
		}

		return int64(int32(binary.LittleEndian.Uint32(raw))) //nolint:gosec // es un int32 con signo en el DBF
	case 'B':
		if len(raw) != 8 {
			return nil
		}

		return math.Float64frombits(binary.LittleEndian.Uint64(raw))
	case 'Y':
		if len(raw) != 8 {
			return nil
		}

		return float64(int64(binary.LittleEndian.Uint64(raw))) / 10000 //nolint:gosec // currency es int64 con signo
	case 'M', 'G', 'W':
		return decodeMemoBlock(raw)
	default:
		text := strings.TrimRight(codepage.Decode(raw), " \x00")
		if text == "" {
			return nil
		}

		return text
	}
}

func decodeNumber(text string, decimals int) any {
	if text == "" || strings.Trim(text, ".-") == "" {
		return nil
	}

	if decimals == 0 {
		if value, err := strconv.ParseInt(text, 10, 64); err == nil {
			return value
		}
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		// Desborde del DBF ("*****") o basura: no se inventa un número.
		return nil
	}

	return value
}

func decodeDate(text string) any {
	parsed, err := time.Parse("20060102", strings.TrimSpace(text))
	if err != nil {
		return nil
	}

	return parsed.Format(time.DateOnly)
}

// decodeDateTime lee un T de Visual FoxPro: día juliano (int32) y
// milisegundos desde medianoche (int32). Se redondea al segundo: Vetter no usa
// milisegundos y FoxPro a veces guarda 999.
func decodeDateTime(raw []byte) any {
	if len(raw) != 8 {
		return nil
	}

	day := int64(binary.LittleEndian.Uint32(raw[0:4]))
	millis := int64(binary.LittleEndian.Uint32(raw[4:8]))

	// Vacío (ceros o espacios) o fuera de 0001-01-01…9999-12-31.
	if day < julianYearOne || day > julianYear9999 || millis >= 86_400_000 {
		return nil
	}

	seconds := (millis + 500) / 1000
	stamp := time.Unix((day-julianUnixEpoch)*86400+seconds, 0).UTC()

	return stamp.Format("2006-01-02T15:04:05")
}

func decodeLogical(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}

	switch raw[0] {
	case 'T', 't', 'Y', 'y':
		return int64(1)
	case 'F', 'f', 'N', 'n':
		return int64(0)
	default:
		return nil
	}
}

// decodeMemoBlock devuelve el número de bloque del .FPT. El texto del memo no
// se lee: en las tablas de la lista blanca no hay memos (inspect 2026-11-18).
func decodeMemoBlock(raw []byte) any {
	var block int64

	if len(raw) == 4 {
		block = int64(binary.LittleEndian.Uint32(raw))
	} else {
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return nil
		}

		block = parsed
	}

	if block == 0 {
		return nil
	}

	return block
}
