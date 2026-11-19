// Package secret guarda y lee el secreto de instalación (vgs_…).
//
// En Windows se cifra con DPAPI scope LocalMachine: lo puede descifrar
// cualquier cuenta de la PC, así que el archivo queda con ACL solo para
// SYSTEM, Administradores y la cuenta que corrió `pair`. Nunca va al repo, a
// OneDrive ni a un argumento de línea de comandos.
package secret

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileName es el nombre del archivo cifrado.
const FileName = "secret.bin"

// Candidates son las rutas donde se guarda, en orden de preferencia:
// %ProgramData%\vetter-agent y, si no hay permiso, %USERPROFILE%\vetter-agent.
func Candidates() []string {
	var out []string

	if dir := os.Getenv("ProgramData"); dir != "" {
		out = append(out, filepath.Join(dir, "vetter-agent", FileName))
	}

	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "vetter-agent", FileName))
	}

	return out
}

// Clean quita lo que agregan consolas y pipes: espacios, fin de línea y el BOM
// que mete PowerShell 5.1 al pasar texto por un pipe.
func Clean(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), string(rune(0xFEFF))))
}

// ReadMasked lee hasta Enter y escribe un * por carácter recibido, incluido
// lo que llega pegado. Backspace borra; otros controles se ignoran. Nunca
// escribe el texto en w.
func ReadMasked(r io.Reader, w io.Writer) (string, error) {
	reader := bufio.NewReader(r)
	value := make([]byte, 0, 64)

	defer func() { _, _ = fmt.Fprintln(w) }()

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if len(value) > 0 && errors.Is(err, io.EOF) {
				return Clean(string(value)), nil
			}

			return "", fmt.Errorf("leer secreto: %w", err)
		}

		switch {
		case b == '\r' || b == '\n':
			return Clean(string(value)), nil
		case b == 0x08 || b == 0x7f:
			if len(value) > 0 {
				value = value[:len(value)-1]
				_, _ = fmt.Fprint(w, "\b \b")
			}
		case b < 0x20:
		default:
			value = append(value, b)
			_, _ = fmt.Fprint(w, "*")
		}
	}
}

// Validate hace chequeos baratos antes de pegarle al servidor.
func Validate(value string) error {
	switch {
	case value == "":
		return errors.New("secreto vacío")
	case strings.ContainsAny(value, " \t\r\n"):
		return errors.New("el secreto no puede tener espacios")
	case !strings.HasPrefix(value, "vgs_"):
		return errors.New("el secreto de instalación empieza con vgs_")
	default:
		return nil
	}
}

// Save cifra el secreto y lo guarda en la primera ruta escribible. Devuelve la
// ruta usada y, si no se pudo restringir la ACL, un aviso no fatal.
func Save(ctx context.Context, value string, paths []string) (string, error) {
	sealed, err := protect([]byte(value))
	if err != nil {
		return "", err
	}

	var problems []error

	for _, path := range paths {
		if err := writeSealed(path, sealed); err != nil {
			problems = append(problems, err)
			continue
		}

		if err := restrict(ctx, path); err != nil {
			return path, fmt.Errorf("guardado en %s pero sin ACL restringida: %w", path, err)
		}

		return path, nil
	}

	return "", fmt.Errorf("no se pudo guardar el secreto: %w", errors.Join(problems...))
}

func writeSealed(path string, sealed []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("crear %s: %w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return fmt.Errorf("escribir %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renombrar %s: %w", tmp, err)
	}

	return nil
}

// Load lee el primer secreto que exista entre las rutas.
func Load(paths []string) (string, string, error) {
	for _, path := range paths {
		sealed, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return "", path, fmt.Errorf("leer %s: %w", path, err)
		}

		plain, err := unprotect(sealed)
		if err != nil {
			return "", path, fmt.Errorf("descifrar %s: %w", path, err)
		}

		return string(plain), path, nil
	}

	return "", "", fmt.Errorf("no hay secreto guardado en %s: correr `vetter-agent pair`", strings.Join(paths, " ni "))
}
