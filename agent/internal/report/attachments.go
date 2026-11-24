package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vetter-sync/agent/internal/dbf"
)

// AttachmentTable es el inventario de una tabla con archivos: ADJUNTOS o FOTOS.
type AttachmentTable struct {
	Rows         int            `json:"filas"`
	Deleted      int            `json:"borradas"`
	WithPath     int            `json:"con_ruta"`
	WithoutPath  int            `json:"sin_ruta"`
	InSource     int            `json:"en_unc"`
	InMirrorOnly int            `json:"solo_en_onedrive"`
	InBoth       int            `json:"en_los_dos"`
	Missing      int            `json:"no_aparece"`
	TotalBytes   int64          `json:"bytes_totales"`
	MaxBytes     int64          `json:"bytes_max"`
	P95Bytes     int64          `json:"bytes_p95"`
	Buckets      map[string]int `json:"por_tamano"`
	Extensions   map[string]int `json:"por_extension"`
	Repeated     int            `json:"rutas_repetidas"`
	Unreadable   int            `json:"ilegibles"`
}

// AttachmentReport es el JSON del comando `attachments`.
type AttachmentReport struct {
	Dir    string                     `json:"dir"`
	Mirror string                     `json:"mirror,omitempty"`
	Tables map[string]AttachmentTable `json:"tables"`
}

// ScanAttachments cuenta los archivos que referencian ADJUNTOS y FOTOS y si
// existen en la fuente (UNC) y en la réplica (OneDrive). Solo agrega: no
// devuelve rutas, títulos ni valores de campo.
func ScanAttachments(dir, mirror string) (AttachmentReport, error) {
	out := AttachmentReport{Dir: dir, Mirror: mirror, Tables: map[string]AttachmentTable{}}

	for _, table := range []string{"ADJUNTOS", "FOTOS"} {
		path, err := findDBF(dir, table)
		if err != nil {
			return out, err
		}

		stats, err := scanTable(path, dir, mirror)
		if err != nil {
			return out, err
		}

		out.Tables[table] = stats
	}

	return out, nil
}

func scanTable(path, dir, mirror string) (AttachmentTable, error) {
	stats := AttachmentTable{Buckets: map[string]int{}, Extensions: map[string]int{}}

	var sizes []int64

	seen := map[string]int{}

	err := dbf.ReadRows(path, func(row dbf.Row) error {
		stats.Rows++

		if row.Deleted {
			stats.Deleted++

			return nil
		}

		raw := strings.TrimSpace(fmt.Sprint(row.Values["ARCHIVO"]))
		if raw == "" || raw == "<nil>" {
			stats.WithoutPath++

			return nil
		}

		stats.WithPath++

		if seen[strings.ToLower(raw)]++; seen[strings.ToLower(raw)] == 2 {
			stats.Repeated++
		}

		stats.Extensions[extension(raw)]++

		sourceSize, sourceErr := sizeIn(dir, raw)
		mirrorSize, mirrorErr := sizeIn(mirror, raw)

		switch {
		case sourceErr == nil && mirrorErr == nil:
			stats.InBoth++
		case sourceErr == nil:
			stats.InSource++
		case mirrorErr == nil:
			stats.InMirrorOnly++
		default:
			stats.Missing++

			if !isNotExist(sourceErr) {
				stats.Unreadable++
			}

			return nil
		}

		size := sourceSize
		if sourceErr != nil {
			size = mirrorSize
		}

		stats.TotalBytes += size

		if size > stats.MaxBytes {
			stats.MaxBytes = size
		}

		stats.Buckets[bucket(size)]++
		sizes = append(sizes, size)

		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("leer %s: %w", path, err)
	}

	stats.P95Bytes = p95(sizes)

	return stats, nil
}

// sizeIn resuelve la ruta relativa contra la carpeta de los DBF y su padre:
// `ARCHIVO` viene con `\` y puede subir un nivel.
func sizeIn(root, raw string) (int64, error) {
	if root == "" {
		return 0, os.ErrNotExist
	}

	relative := strings.ReplaceAll(raw, "\\", string(filepath.Separator))
	relative = strings.TrimPrefix(relative, string(filepath.Separator))

	candidates := []string{
		filepath.Join(root, relative),
		filepath.Join(filepath.Dir(root), relative),
	}

	var last error

	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Clean(candidate))
		if err == nil {
			if info.IsDir() {
				continue
			}

			return info.Size(), nil
		}

		last = err
	}

	return 0, last
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func extension(path string) string {
	ext := strings.ToLower(filepath.Ext(strings.ReplaceAll(path, "\\", "/")))

	return strings.TrimPrefix(ext, ".")
}

func bucket(size int64) string {
	switch {
	case size <= 1<<20:
		return "hasta_1MB"
	case size <= 5<<20:
		return "1_5MB"
	case size <= 16<<20:
		return "5_16MB"
	case size <= 50<<20:
		return "16_50MB"
	case size <= 100<<20:
		return "50_100MB"
	default:
		return "mas_100MB"
	}
}

func p95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return sorted[len(sorted)*95/100]
}

func findDBF(dir, table string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("listar %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), table+".dbf") {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no está %s.dbf en %s", table, dir)
}
