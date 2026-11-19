package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func signed(t *testing.T, manifest []byte) (string, []byte) {
	t.Helper()

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(public), ed25519.Sign(private, manifest)
}

func TestDecode(t *testing.T) {
	t.Parallel()

	manifest := []byte(`{"version":"0.2.0","url":"https://github.com/x/vetter-agent.exe","sha256":"abc","min_contract":3}`)
	key, signature := signed(t, manifest)

	release, err := Decode(manifest, signature, key)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if release.Version != "0.2.0" || release.SHA256 != "abc" {
		t.Errorf("release = %+v", release)
	}

	// Un byte distinto en el manifiesto invalida la firma.
	tampered := append([]byte{}, manifest...)
	tampered[len(tampered)-2] = 'X'

	if _, err := Decode(tampered, signature, key); err == nil {
		t.Error("un manifiesto alterado validó")
	}

	if _, err := Decode(manifest, signature, "no-es-base64"); err == nil {
		t.Error("una clave inválida validó")
	}

	if _, err := Decode(manifest, signature[:10], key); err == nil {
		t.Error("una firma corta validó")
	}
}

func TestDecodeRechazaURLSinHTTPS(t *testing.T) {
	t.Parallel()

	manifest := []byte(`{"version":"0.2.0","url":"http://example.com/a.exe","sha256":"abc"}`)
	key, signature := signed(t, manifest)

	if _, err := Decode(manifest, signature, key); err == nil {
		t.Error("un manifiesto por http validó")
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"0.2.0", "0.1.9", true},
		{"0.2.0", "0.2.0-dev", true},
		{"0.2.0", "0.2.0", false},
		{"0.1.9", "0.2.0", false},
		{"0.10.0", "0.9.9", true},
		{"v0.3.0", "0.2.9", true},
		{"1.0.0", "0.99.99", true},
	}

	for _, c := range cases {
		if got := Newer(c.candidate, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, quería %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestDownloadVerificaSHA(t *testing.T) {
	t.Parallel()

	body := []byte("ejecutable")
	sum := sha256.Sum256(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	dest := filepath.Join(t.TempDir(), "vetter-agent.exe.new")

	ok := Release{URL: server.URL, SHA256: hex.EncodeToString(sum[:])}

	if err := Download(context.Background(), server.Client(), ok, "vetter-agent.exe", dest); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("no quedó el archivo: %v", err)
	}

	bad := Release{URL: server.URL, SHA256: "00" + hex.EncodeToString(sum[:])[2:]}

	if err := Download(context.Background(), server.Client(), bad, "vetter-agent.exe", dest); err == nil {
		t.Fatal("un sha256 distinto pasó")
	}

	if _, err := os.Stat(dest); err == nil {
		t.Error("el archivo malo quedó en disco")
	}
}

func TestRotateYRestore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exe := filepath.Join(dir, "vetter-agent.exe")

	if err := os.WriteFile(exe, []byte("viejo"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(exe+".new", []byte("nuevo"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Rotate(exe); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertContent(t, exe, "nuevo")
	assertContent(t, exe+".old", "viejo")

	if err := Restore(exe); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	assertContent(t, exe, "viejo")
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}

	if string(got) != want {
		t.Errorf("%s = %q, quería %q", path, got, want)
	}
}

func TestAssetEligeElBinarioDeLaTarea(t *testing.T) {
	t.Parallel()

	release := Release{
		URL:        "https://x/vetter-agent.exe",
		SHA256:     "consola",
		TaskURL:    "https://x/vetter-agent-task.exe",
		TaskSHA256: "tarea",
	}

	if url, sha := release.Asset(`C:\ProgramData\vetter-agent\vetter-agent-task.exe`); url != release.TaskURL || sha != "tarea" {
		t.Errorf("tarea: %q %q", url, sha)
	}

	if url, sha := release.Asset(`C:\ProgramData\vetter-agent\vetter-agent.exe`); url != release.URL || sha != "consola" {
		t.Errorf("consola: %q %q", url, sha)
	}

	// Un release viejo sin task_url no rompe: baja el de consola.
	viejo := Release{URL: release.URL, SHA256: "consola"}

	if url, _ := viejo.Asset("vetter-agent-task.exe"); url != viejo.URL {
		t.Errorf("sin task_url: %q", url)
	}
}

func TestCheckedAt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "update.checked")

	if !CheckedAt(path).IsZero() {
		t.Error("sin archivo debería ser cero")
	}

	now := time.Now().UTC().Truncate(time.Second)

	if err := MarkChecked(path, now); err != nil {
		t.Fatal(err)
	}

	if got := CheckedAt(path); !got.Equal(now) {
		t.Errorf("CheckedAt = %v, quería %v", got, now)
	}
}
