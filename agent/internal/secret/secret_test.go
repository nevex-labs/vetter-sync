package secret

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadMasked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want, echo string
	}{
		{name: "tipeado", in: "vgs_ab\r", want: "vgs_ab", echo: "******\n"},
		{name: "pegado con CRLF", in: "vgs_ab\r\nresto", want: "vgs_ab", echo: "******\n"},
		{name: "backspace", in: "vgs_abX\bc\r", want: "vgs_abc", echo: "*******\b \b*\n"},
		{name: "controles ignorados", in: "vgs_\x16ab\n", want: "vgs_ab", echo: "******\n"},
		{name: "EOF sin Enter", in: "vgs_ab", want: "vgs_ab", echo: "******\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var echo strings.Builder

			got, err := ReadMasked(strings.NewReader(tt.in), &echo)
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.want || echo.String() != tt.echo {
				t.Errorf("ReadMasked = %q, eco %q; quería %q, eco %q", got, echo.String(), tt.want, tt.echo)
			}

			if strings.Contains(echo.String(), "vgs_") {
				t.Error("el eco no puede mostrar el secreto")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		ok    bool
	}{
		{name: "válido", value: "vgs_abc123", ok: true},
		{name: "vacío", value: ""},
		{name: "sin prefijo", value: "abc123"},
		{name: "con espacio", value: "vgs_abc 123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := Validate(tt.value); (err == nil) != tt.ok {
				t.Errorf("Validate(%q) = %v", tt.value, err)
			}
		})
	}
}

func TestClean(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{in: "vgs_a\r\n", want: "vgs_a"},
		{in: string(rune(0xFEFF)) + "vgs_a\n", want: "vgs_a"},
		{in: "  vgs_a  ", want: "vgs_a"},
		{in: "\"vgs_a\"", want: "vgs_a"},
		{in: "vgs_a\r\nvgs_a", want: "vgs_avgs_a"},
		{in: "vgs_\u200Ba", want: "vgs_a"},
		{in: "'vgs_a'\n", want: "vgs_a"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean(%q) = %q", tt.in, got)
			}
		})
	}
}

// Ida y vuelta por DPAPI con un secreto falso. Si la primera ruta no se puede
// crear, cae en la segunda.
func TestSaveLoadFallback(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("DPAPI solo existe en Windows")
	}

	dir := t.TempDir()
	blocker := filepath.Join(dir, "no-es-carpeta")

	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	paths := []string{filepath.Join(blocker, FileName), filepath.Join(dir, "ok", FileName)}

	used, err := Save(context.Background(), "vgs_falso_de_test", paths)
	if err != nil {
		t.Fatal(err)
	}

	if used != paths[1] {
		t.Errorf("usó %s, quería el fallback %s", used, paths[1])
	}

	sealed, err := os.ReadFile(used)
	if err != nil {
		t.Fatal(err)
	}

	if string(sealed) == "vgs_falso_de_test" {
		t.Error("el secreto quedó en claro")
	}

	got, from, err := Load(paths)
	if err != nil || got != "vgs_falso_de_test" || from != used {
		t.Errorf("Load = %q desde %s, err %v", got, from, err)
	}
}

func TestLoadMissing(t *testing.T) {
	t.Parallel()

	if _, _, err := Load([]string{filepath.Join(t.TempDir(), FileName)}); err == nil {
		t.Error("sin archivo tiene que pedir `pair`")
	}
}
