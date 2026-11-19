package syncer

import "testing"

// Vectores de docs/agent-implementation.md. No se inventan: si cambian, cambia
// el contrato.
func TestHashVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    map[string]any
		deleted bool
		want    string
	}{
		{
			name:    "tipos y null",
			data:    map[string]any{"FECHA": "2026-10-14", "NOMBRE": "Toby", "PESO": 12.5, "VACIO": nil},
			deleted: false,
			want:    "7ad0011f65b051a4155338551a016e8b247b20bd28ee8dd275cdd6f88ee53d7a",
		},
		{
			name:    "UTF-8 sin escapar",
			data:    map[string]any{"NOMBRE": "Ñandú", "TUTOR": "Pérez"},
			deleted: false,
			want:    "83772c3537ce0dc66594750f4e49d6a33b0f690717bea54cc5a5494b69db6050",
		},
		{
			name:    "borrado",
			data:    map[string]any{"CODIGO": "1524"},
			deleted: true,
			want:    "7cbf9dce123746c03d5478341fb84ef4e9b2b6b10552b941aeb54d15d1fa90b6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Hash(tt.data, tt.deleted)
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.want {
				t.Errorf("hash = %s\nquería %s", got, tt.want)
			}
		})
	}
}

func TestHashDoesNotMutateData(t *testing.T) {
	t.Parallel()

	data := map[string]any{"CODIGO": int64(1)}

	if _, err := Hash(data, true); err != nil {
		t.Fatal(err)
	}

	if _, ok := data["deleted"]; ok {
		t.Error("Hash no puede agregar deleted a data: viaja al Worker tal cual")
	}
}
