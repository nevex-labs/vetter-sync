// Package dbf lee la estructura de un DBF/FoxPro sin interpretar los valores:
// header, campos, cantidad de registros y cuántos están marcados como borrados.
//
// Solo lectura. Nunca abre un archivo para escritura: el DBF de Vetter es
// producción y escribir ahí es corrupción.
package dbf

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Header es el bloque de 32 bytes al principio del archivo.
type Header struct {
	Version        byte
	UpdatedAt      string
	Records        int
	HeaderBytes    int
	RecordBytes    int
	LanguageDriver byte
}

// Field es un descriptor de campo de 32 bytes.
type Field struct {
	Name     string
	Type     byte
	Length   int
	Decimals int
}

// Table es lo que se puede saber sin leer el contenido de los registros.
type Table struct {
	Path     string
	Size     int64
	Header   Header
	Fields   []Field
	Deleted  int
	Scanned  int
	Computed int
}

// Stats son agregados de un campo. No incluye valores: solo largos y
// separadores, para no sacar PII de la clínica.
type Stats struct {
	Field         string
	Scanned       int
	NonEmpty      int
	DigitsOnly    int
	HasSpace      int
	HasComma      int
	HasSlash      int
	HasPlus       int
	HasParen      int
	HasDash       int
	Lengths       map[int]int
	PrefixLengths map[int]int
}

var encodings = map[byte]string{
	0x01: "CP437",
	0x02: "CP850",
	0x03: "CP1252",
	0x57: "ANSI (CP1252)",
	0x64: "CP852",
	0x65: "CP865",
	0x66: "CP866",
	0x67: "CP861",
	0x6a: "CP737",
	0x6b: "CP857",
	0xc8: "CP1250",
	0xc9: "CP1251",
	0xca: "CP1254",
	0xcb: "CP1253",
	0xcc: "CP1257",
}

// Encoding devuelve una hipótesis legible para el language driver del header.
func Encoding(driver byte) string {
	if name, ok := encodings[driver]; ok {
		return name
	}

	return "desconocido"
}

// Open lee el header, los campos y cuenta los registros borrados.
func Open(path string) (*Table, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	head, err := readHeader(file)
	if err != nil {
		return nil, err
	}

	fields, err := readFields(file, head.HeaderBytes)
	if err != nil {
		return nil, err
	}

	table := &Table{
		Path:     path,
		Size:     info.Size(),
		Header:   head,
		Fields:   fields,
		Computed: computedRecords(info.Size(), head),
	}

	if err := scanDeleted(file, table); err != nil {
		return nil, err
	}

	return table, nil
}

func readHeader(file *os.File) (Header, error) {
	buf := make([]byte, 32)

	if _, err := io.ReadFull(file, buf); err != nil {
		return Header{}, fmt.Errorf("header: %w", err)
	}

	if buf[0] == 0 {
		return Header{}, errors.New("no parece un DBF: byte de versión 0")
	}

	return Header{
		Version:        buf[0],
		UpdatedAt:      fmt.Sprintf("%04d-%02d-%02d", int(buf[1])+1900, buf[2], buf[3]),
		Records:        int(binary.LittleEndian.Uint32(buf[4:8])),
		HeaderBytes:    int(binary.LittleEndian.Uint16(buf[8:10])),
		RecordBytes:    int(binary.LittleEndian.Uint16(buf[10:12])),
		LanguageDriver: buf[29],
	}, nil
}

func readFields(file *os.File, headerBytes int) ([]Field, error) {
	if headerBytes < 33 {
		return nil, fmt.Errorf("header inválido: %d bytes", headerBytes)
	}

	// Los descriptores empiezan en el byte 32 y terminan con 0x0D.
	end := headerBytes - 1

	fields := make([]Field, 0, 16)
	buf := make([]byte, 32)

	for offset := 32; offset+32 <= end; offset += 32 {
		if _, err := io.ReadFull(file, buf); err != nil {
			return nil, fmt.Errorf("campo en offset %d: %w", offset, err)
		}

		if buf[0] == 0x0d {
			break
		}

		fields = append(fields, Field{
			Name:     strings.TrimRight(string(buf[0:11]), "\x00 "),
			Type:     buf[11],
			Length:   int(buf[16]),
			Decimals: int(buf[17]),
		})
	}

	return fields, nil
}

func computedRecords(size int64, head Header) int {
	if head.RecordBytes <= 0 {
		return 0
	}

	rest := size - int64(head.HeaderBytes)

	if rest < 0 {
		return 0
	}

	return int(rest / int64(head.RecordBytes))
}

// scanDeleted recorre los registros leyendo solo el flag de borrado del
// primero byte. No decodifica ningún valor.
func scanDeleted(file *os.File, table *Table) error {
	head := table.Header

	if head.Records <= 0 || head.RecordBytes <= 0 || head.HeaderBytes <= 0 {
		return nil
	}

	if _, err := file.Seek(int64(head.HeaderBytes), io.SeekStart); err != nil {
		return fmt.Errorf("seek %s: %w", table.Path, err)
	}

	reader := bufio.NewReaderSize(file, 64*1024)

	for i := 0; i < head.Records; i++ {
		flag, err := reader.ReadByte()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("registro %d: %w", i, err)
		}

		table.Scanned++

		if flag == '*' {
			table.Deleted++
		}

		if _, err := reader.Discard(head.RecordBytes - 1); err != nil && err != io.EOF {
			return fmt.Errorf("registro %d: %w", i, err)
		}
	}

	return nil
}

// Inspect lee un campo de todos los registros y devuelve agregados. Los
// borrados no cuentan: están lógicamente fuera.
func (t *Table) Inspect(fieldName string) (Stats, error) {
	index := -1

	for i, field := range t.Fields {
		if strings.EqualFold(field.Name, fieldName) {
			index = i
			break
		}
	}

	if index < 0 {
		return Stats{}, fmt.Errorf("no existe el campo %s en %s", fieldName, t.Path)
	}

	offset := 1

	for _, field := range t.Fields[:index] {
		offset += field.Length
	}

	length := t.Fields[index].Length
	stats := Stats{
		Field:         t.Fields[index].Name,
		Lengths:       map[int]int{},
		PrefixLengths: map[int]int{},
	}

	file, err := os.Open(t.Path)
	if err != nil {
		return Stats{}, fmt.Errorf("abrir %s: %w", t.Path, err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Seek(int64(t.Header.HeaderBytes), io.SeekStart); err != nil {
		return Stats{}, fmt.Errorf("seek %s: %w", t.Path, err)
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	record := make([]byte, t.Header.RecordBytes)

	for i := 0; i < t.Header.Records; i++ {
		if _, err := io.ReadFull(reader, record); err != nil {
			break
		}

		if record[0] == '*' {
			continue
		}

		if offset+length > len(record) {
			break
		}

		analyze(&stats, strings.TrimRight(string(record[offset:offset+length]), "\x00 "))
	}

	return stats, nil
}

// analyze suma un valor a los agregados sin guardarlo.
func analyze(stats *Stats, value string) {
	stats.Scanned++

	if value == "" {
		return
	}

	stats.NonEmpty++
	stats.Lengths[len(value)]++

	digits := 0

	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ' ':
			stats.HasSpace++
		case ',':
			stats.HasComma++
		case '/':
			stats.HasSlash++
		case '+':
			stats.HasPlus++
		case '(':
			stats.HasParen++
		case '-':
			stats.HasDash++
		}

		if value[i] >= '0' && value[i] <= '9' {
			digits++
		}
	}

	if digits == len(value) {
		stats.DigitsOnly++
	}

	if at := strings.IndexByte(value, '/'); at >= 0 {
		stats.PrefixLengths[at]++
	}
}
