// Package report arma el JSON que se lleva de la visita a la PC.
//
// No incluye valores de campos: nombres, tamaños, conteos y agregados. Nada de
// nombres de tutores, teléfonos ni notas.
package report

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vetter-sync/agent/internal/dbf"
)

// StatSpec pide agregados de un campo: "CLIENTES.TELEFONO".
type StatSpec struct {
	Table string
	Field string
}

// ParseStat convierte "TABLA.CAMPO" en un StatSpec.
func ParseStat(spec string) (StatSpec, error) {
	parts := strings.Split(spec, ".")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return StatSpec{}, fmt.Errorf("stat inválido %q: usá TABLA.CAMPO", spec)
	}

	return StatSpec{Table: parts[0], Field: parts[1]}, nil
}

type Report struct {
	GeneratedAt string      `json:"generated_at"`
	Dir         string      `json:"dir"`
	Files       []File      `json:"files"`
	Errors      []FileError `json:"errors"`
}

type FileError struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

type File struct {
	Name     string      `json:"name"`
	Size     int64       `json:"size"`
	Modified string      `json:"modified"`
	Memo     *Memo       `json:"memo"`
	Header   Header      `json:"header"`
	Records  Records     `json:"records"`
	Fields   []Field     `json:"fields"`
	Stats    []StatBlock `json:"stats"`
}

type Memo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Header struct {
	Version        string `json:"version"`
	UpdatedAt      string `json:"updated_at"`
	HeaderBytes    int    `json:"header_bytes"`
	RecordBytes    int    `json:"record_bytes"`
	LanguageDriver string `json:"language_driver"`
	Encoding       string `json:"encoding"`
}

type Records struct {
	Header   int  `json:"header"`
	Computed int  `json:"computed_from_size"`
	Match    bool `json:"match"`
	Deleted  int  `json:"deleted"`
}

type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Length   int    `json:"length"`
	Decimals int    `json:"decimals"`
}

type StatBlock struct {
	Field         string         `json:"field"`
	Scanned       int            `json:"scanned"`
	NonEmpty      int            `json:"non_empty"`
	DigitsOnly    int            `json:"digits_only"`
	Separators    map[string]int `json:"separators"`
	Lengths       map[string]int `json:"lengths"`
	PrefixLengths map[string]int `json:"prefix_lengths"`
}

// Build recorre el directorio y arma el reporte. Los archivos que no se pueden
// abrir se listan en `errors` y el resto sigue.
func Build(dir string, specs []StatSpec) (Report, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Report{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return Report{}, err
	}

	if !info.IsDir() {
		return Report{}, fmt.Errorf("%s no es un directorio", abs)
	}

	paths := make([]string, 0, 16)

	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		if strings.EqualFold(filepath.Ext(path), ".dbf") {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		return Report{}, err
	}

	sort.Strings(paths)

	out := Report{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Dir:         abs,
		Files:       make([]File, 0, len(paths)),
		Errors:      []FileError{},
	}

	matched := map[string]bool{}

	for _, path := range paths {
		table, err := dbf.Open(path)
		if err != nil {
			out.Errors = append(out.Errors, FileError{File: filepath.Base(path), Error: err.Error()})
			continue
		}

		file, err := buildFile(table, specs, matched)
		if err != nil {
			out.Errors = append(out.Errors, FileError{File: filepath.Base(path), Error: err.Error()})
			continue
		}

		out.Files = append(out.Files, file)
	}

	for _, spec := range specs {
		if !matched[strings.ToUpper(spec.Table)] {
			out.Errors = append(out.Errors, FileError{
				File:  spec.Table,
				Error: "el --stat no coincide con ningún DBF del directorio",
			})
		}
	}

	return out, nil
}

func buildFile(table *dbf.Table, specs []StatSpec, matched map[string]bool) (File, error) {
	info, err := os.Stat(table.Path)
	if err != nil {
		return File{}, err
	}

	base := strings.TrimSuffix(filepath.Base(table.Path), filepath.Ext(table.Path))

	file := File{
		Name:     filepath.Base(table.Path),
		Size:     info.Size(),
		Modified: info.ModTime().Format(time.RFC3339),
		Header: Header{
			Version:        fmt.Sprintf("0x%02X", table.Header.Version),
			UpdatedAt:      table.Header.UpdatedAt,
			HeaderBytes:    table.Header.HeaderBytes,
			RecordBytes:    table.Header.RecordBytes,
			LanguageDriver: fmt.Sprintf("0x%02X", table.Header.LanguageDriver),
			Encoding:       dbf.Encoding(table.Header.LanguageDriver),
		},
		Records: Records{
			Header:   table.Header.Records,
			Computed: table.Computed,
			Match:    table.Header.Records == table.Computed,
			Deleted:  table.Deleted,
		},
		Fields: make([]Field, 0, len(table.Fields)),
		Stats:  []StatBlock{},
	}

	if memoPath := strings.TrimSuffix(table.Path, filepath.Ext(table.Path)) + ".FPT"; memoExists(memoPath) {
		if memoInfo, err := os.Stat(memoPath); err == nil {
			file.Memo = &Memo{Name: filepath.Base(memoPath), Size: memoInfo.Size()}
		}
	}

	for _, field := range table.Fields {
		file.Fields = append(file.Fields, Field{
			Name:     field.Name,
			Type:     string(field.Type),
			Length:   field.Length,
			Decimals: field.Decimals,
		})
	}

	for _, spec := range specs {
		if !strings.EqualFold(spec.Table, base) {
			continue
		}

		stats, err := table.Inspect(spec.Field)
		if err != nil {
			return File{}, err
		}

		matched[strings.ToUpper(spec.Table)] = true
		file.Stats = append(file.Stats, toBlock(stats))
	}

	return file, nil
}

func memoExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func toBlock(stats dbf.Stats) StatBlock {
	block := StatBlock{
		Field:         stats.Field,
		Scanned:       stats.Scanned,
		NonEmpty:      stats.NonEmpty,
		DigitsOnly:    stats.DigitsOnly,
		Separators:    map[string]int{},
		Lengths:       toStringMap(stats.Lengths),
		PrefixLengths: toStringMap(stats.PrefixLengths),
	}

	for name, count := range map[string]int{
		"space": stats.HasSpace,
		"comma": stats.HasComma,
		"slash": stats.HasSlash,
		"plus":  stats.HasPlus,
		"paren": stats.HasParen,
		"dash":  stats.HasDash,
	} {
		if count > 0 {
			block.Separators[name] = count
		}
	}

	return block
}

func toStringMap(values map[int]int) map[string]int {
	out := make(map[string]int, len(values))

	for key, value := range values {
		out[fmt.Sprintf("%d", key)] = value
	}

	return out
}

// Write guarda el reporte como JSON legible.
func Write(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o600)
}
