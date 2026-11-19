package syncer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
		check   func(*testing.T, Config)
	}{
		{
			name: "completo con defaults",
			body: `{"z_path":"\\\\srv\\Datos","copy_dir":"C:\\x\\copy","snapshot":"C:\\x\\snapshot.json",
				"endpoint":"https://w/api/sync/vetter","install_id":"pc-vete","keys":{"CLIENTES":"CODIGO"}}`,
			check: func(t *testing.T, cfg Config) {
				t.Helper()

				if cfg.MaxRowsPerRun != 5000 || cfg.Keys["CLIENTES"] != "CODIGO" || cfg.LogDir == "" {
					t.Errorf("config = %+v", cfg)
				}
			},
		},
		{name: "faltan campos", body: `{"z_path":"x"}`, wantErr: "falta endpoint"},
		{name: "JSON roto", body: `{`, wantErr: "unexpected end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := LoadConfig(path)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, quería %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			tt.check(t, cfg)
		})
	}
}
