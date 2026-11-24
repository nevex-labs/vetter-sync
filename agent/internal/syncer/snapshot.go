package syncer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileStamp es lo que se compara para saltear una tabla que no cambió.
type FileStamp struct {
	Size  int64 `json:"size"`
	MTime int64 `json:"mtime"`
}

// Snapshot es el estado confirmado por el Worker: el hash de cada fila
// ("TABLA|row_key"), el (size, mtime) del DBF de la última tabla que quedó
// completa y la versión del agente que lo llenó.
//
// La versión importa: cuando el agente cambia la forma de nombrar las filas
// (`row_key`), las claves nuevas no están en el snapshot y una tabla sin cambios
// no se lee, así que esas filas quedarían sin mandar hasta que Vetter toque el
// DBF. Con la versión guardada, la primera corrida después de un upgrade lee
// todas las tablas una vez.
type Snapshot struct {
	Version string               `json:"version"`
	Rows    map[string]string    `json:"rows"`
	Files   map[string]FileStamp `json:"files"`
}

func rowKey(table, vetterID string) string {
	return table + "|" + vetterID
}

// LoadSnapshot lee el snapshot. Si no existe, la primera corrida manda todo.
func LoadSnapshot(path string) (Snapshot, error) {
	snap := Snapshot{Rows: map[string]string{}, Files: map[string]FileStamp{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return snap, nil
	}

	if err != nil {
		return snap, fmt.Errorf("leer snapshot %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, fmt.Errorf("snapshot %s: %w", path, err)
	}

	if snap.Rows == nil {
		snap.Rows = map[string]string{}
	}

	if snap.Files == nil {
		snap.Files = map[string]FileStamp{}
	}

	return snap, nil
}

// Save escribe el snapshot de forma atómica: temporal y rename.
func (s *Snapshot) Save(path string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("serializar snapshot: %w", err)
	}

	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("crear %s: %w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("escribir %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renombrar %s: %w", tmp, err)
	}

	return nil
}
