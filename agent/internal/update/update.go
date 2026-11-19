// Package update baja la versión nueva del agente desde el release firmado.
//
// El manifiesto y su firma viven en el release "latest" de GitHub. El binario
// compila dentro de sí la clave pública (ed25519): si la firma no valida, no se
// toca nada. Después se verifica el sha256 del ejecutable descargado.
//
// En Windows no se puede sobrescribir un .exe en ejecución, pero sí renombrarlo:
// el actual pasa a `.old`, el nuevo ocupa su lugar y la corrida siguiente ya es
// la versión nueva. Si la nueva falla tres corridas seguidas, se restaura el
// `.old`.
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// ManifestURL y SignatureURL son fijas: apuntan al release "latest".
	ManifestURL = "https://github.com/nevex-labs/vetter-sync/releases/latest/download/manifest.json"
	// SignatureURL es la firma ed25519 de ManifestURL.
	SignatureURL = "https://github.com/nevex-labs/vetter-sync/releases/latest/download/manifest.sig"

	// maxManifest es el tope del manifiesto y la firma: unos cientos de bytes.
	maxManifest = 8 * 1024

	// maxBinary es el tope del ejecutable: hoy pesa ~12 MB.
	maxBinary = 64 * 1024 * 1024
)

// Release es el manifiesto firmado.
type Release struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	PublishedAt string `json:"published_at"`
	MinContract int    `json:"min_contract"`
}

// Decode valida la firma y devuelve el manifiesto. Sin una clave de 32 bytes
// no hay actualización: un binario sin clave no se actualiza solo.
func Decode(manifest, signature []byte, publicKey string) (Release, error) {
	var release Release

	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return release, errors.New("clave pública inválida")
	}

	if len(signature) != ed25519.SignatureSize {
		return release, errors.New("firma de largo inválido")
	}

	if !ed25519.Verify(ed25519.PublicKey(key), manifest, signature) {
		return release, errors.New("la firma no valida")
	}

	if err := json.Unmarshal(manifest, &release); err != nil {
		return release, fmt.Errorf("manifiesto ilegible: %w", err)
	}

	if release.Version == "" || release.SHA256 == "" {
		return release, errors.New("manifiesto incompleto")
	}

	if !strings.HasPrefix(release.URL, "https://") {
		return release, errors.New("el manifiesto no apunta a https")
	}

	return release, nil
}

// Newer dice si `candidate` es una versión mayor que `current`. Compara los
// números y, si empatan, un `candidate` sin sufijo le gana a uno con sufijo
// ("0.2.0" > "0.2.0-dev").
func Newer(candidate, current string) bool {
	a, apre := splitVersion(candidate)
	b, bpre := splitVersion(current)

	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}

	return apre == "" && bpre != ""
}

func splitVersion(v string) ([3]int, string) {
	var numbers [3]int

	core, pre, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")

	for i, part := range strings.SplitN(core, ".", 3) {
		if i > 2 {
			break
		}

		n, _ := strconv.Atoi(strings.TrimSpace(part))
		numbers[i] = n
	}

	return numbers, pre
}

// Fetch trae el manifiesto y la firma del último release.
func Fetch(ctx context.Context, client *http.Client) ([]byte, []byte, error) {
	manifest, err := get(ctx, client, ManifestURL, maxManifest)
	if err != nil {
		return nil, nil, fmt.Errorf("manifiesto: %w", err)
	}

	signature, err := get(ctx, client, SignatureURL, maxManifest)
	if err != nil {
		return nil, nil, fmt.Errorf("firma: %w", err)
	}

	return manifest, signature, nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("armar pedido: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pedir %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("leer respuesta: %w", err)
	}

	return data, nil
}

// Download baja el binario a `dest` y verifica el sha256 del manifiesto antes
// de dejarlo: si no coincide, borra el archivo. Se hashea al vuelo, sin tener
// el ejecutable entero en memoria.
func Download(ctx context.Context, client *http.Client, release Release, dest string) error {
	// El binario tiene que poder ejecutarse: en Windows el modo no importa, en
	// el resto sí.
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o750) //nolint:gosec
	if err != nil {
		return fmt.Errorf("crear %s: %w", dest, err)
	}

	sum := sha256.New()

	written, err := download(ctx, client, release.URL, io.MultiWriter(file, sum))
	closeErr := file.Close()

	if err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(dest)

		return err
	}

	if written == 0 || written >= maxBinary {
		_ = os.Remove(dest)

		return errors.New("el ejecutable vino vacío o demasiado grande")
	}

	if hex.EncodeToString(sum.Sum(nil)) != strings.ToLower(release.SHA256) {
		_ = os.Remove(dest)

		return errors.New("el sha256 no coincide")
	}

	return nil
}

func download(ctx context.Context, client *http.Client, url string, dest io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("armar pedido: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("pedir %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	written, err := io.Copy(dest, io.LimitReader(resp.Body, maxBinary))
	if err != nil {
		return written, fmt.Errorf("leer ejecutable: %w", err)
	}

	return written, nil
}

// Rotate deja `path` con el contenido de `path+".new"` y guarda el anterior en
// `path+".old"`. En Windows el .exe en ejecución se puede renombrar.
func Rotate(path string) error {
	previous := path + ".old"

	_ = os.Remove(previous)

	if err := os.Rename(path, previous); err != nil {
		return fmt.Errorf("renombrar el actual: %w", err)
	}

	if err := os.Rename(path+".new", path); err != nil {
		// Dejar el binario viejo en su lugar: es lo único que garantiza que la
		// próxima corrida siga funcionando.
		_ = os.Rename(previous, path)

		return fmt.Errorf("poner el nuevo: %w", err)
	}

	return nil
}

// Restore vuelve el `.old` si existe y borra el contador de intentos.
func Restore(path string) error {
	previous := path + ".old"

	if _, err := os.Stat(previous); err != nil {
		return fmt.Errorf("no hay versión anterior: %w", err)
	}

	if err := os.Rename(path, path+".bad"); err != nil {
		_ = os.Remove(path) // el que falla no sirve
	}

	if err := os.Rename(previous, path); err != nil {
		return fmt.Errorf("volver a la anterior: %w", err)
	}

	return nil
}

// CheckedAt lee la marca de la última consulta al release.
func CheckedAt(path string) time.Time {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}

	at, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}
	}

	return at
}

// MarkChecked guarda cuándo se consultó el release.
func MarkChecked(path string, at time.Time) error {
	if err := os.WriteFile(path, []byte(at.UTC().Format(time.RFC3339)), 0o600); err != nil {
		return fmt.Errorf("marcar la consulta: %w", err)
	}

	return nil
}
