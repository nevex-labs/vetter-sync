package syncer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// findTable busca TABLA.DBF sin importar mayúsculas: en la clínica conviven
// CLIENTES.DBF y visitas.dbf.
func findTable(dir, table string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("listar %s: %w", dir, err)
	}

	want := table + ".dbf"

	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), want) {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no está %s en %s", want, dir)
}

func stampOf(path string) (FileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileStamp{}, fmt.Errorf("stat %s: %w", path, err)
	}

	return FileStamp{Size: info.Size(), MTime: info.ModTime().UnixNano()}, nil
}

// copied es una copia local lista para parsear.
type copied struct {
	Path   string
	Stamp  FileStamp
	Stable bool
}

// copyTable copia el DBF a copyDir con nombre temporal y rename. El original
// se abre solo para lectura (en Windows, os.Open comparte lectura y escritura,
// así que Vetter puede seguir escribiendo). Si el mtime cambió durante la
// copia, reintenta una vez; si sigue cambiando, la copia se usa pero se marca
// inestable para no dar la tabla por cerrada.
func copyTable(src, copyDir, table string) (copied, error) {
	if err := os.MkdirAll(copyDir, 0o750); err != nil {
		return copied{}, fmt.Errorf("crear %s: %w", copyDir, err)
	}

	dst := filepath.Join(copyDir, table+".DBF")

	var result copied

	for attempt := 0; attempt < 2; attempt++ {
		before, err := stampOf(src)
		if err != nil {
			return copied{}, err
		}

		if err := copyFile(src, dst); err != nil {
			return copied{}, err
		}

		after, err := stampOf(src)
		if err != nil {
			return copied{}, err
		}

		result = copied{Path: dst, Stamp: before, Stable: before == after}
		if result.Stable {
			break
		}
	}

	return result, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	tmp := dst + ".tmp"

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("crear %s: %w", tmp, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copiar %s: %w", src, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("cerrar %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("renombrar %s: %w", tmp, err)
	}

	return nil
}
