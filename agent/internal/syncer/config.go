// Package syncer es la corrida de la tarea programada: copia los DBF de la
// lista blanca, los parsea, hashea cada fila y empuja lo que cambió al Worker.
//
// No mapea a tutors, pets, visits ni preventives: eso vive en el Worker.
package syncer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Tables es la lista blanca del contrato, en el orden en que se mandan.
func Tables() []string {
	return []string{"CLIENTES", "PACIENTES", "VACUNAS", "VISITAS", "ADJUNTOS"}
}

// Config es el config.json de la PC. Vive fuera del repo.
type Config struct {
	ZPath string `json:"z_path"`
	// MirrorPath es la carpeta donde el Vetter de esta PC escribe (OneDrive).
	// Opcional: si está, cada corrida compara cuántos registros tiene cada tabla
	// contra el UNC y avisa si divergen. Solo lee headers.
	MirrorPath    string            `json:"mirror_path"`
	CopyDir       string            `json:"copy_dir"`
	Snapshot      string            `json:"snapshot"`
	Endpoint      string            `json:"endpoint"`
	InstallID     string            `json:"install_id"`
	MaxRowsPerRun int               `json:"max_rows_per_run"`
	Keys          map[string]string `json:"keys"`
	LogDir        string            `json:"log_dir"`
}

// LoadConfig lee y valida el config.json.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("leer config %s: %w", path, err)
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}

	if cfg.MaxRowsPerRun <= 0 {
		cfg.MaxRowsPerRun = 5000
	}

	if cfg.LogDir == "" && cfg.Snapshot != "" {
		cfg.LogDir = filepath.Join(filepath.Dir(cfg.Snapshot), "logs")
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var problems []error

	required := []struct{ name, value string }{
		{"z_path", c.ZPath},
		{"copy_dir", c.CopyDir},
		{"snapshot", c.Snapshot},
		{"endpoint", c.Endpoint},
		{"install_id", c.InstallID},
	}

	for _, item := range required {
		if item.value == "" {
			problems = append(problems, fmt.Errorf("falta %s", item.name))
		}
	}

	return errors.Join(problems...)
}
