//go:build windows

package secret

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"unsafe"
)

const (
	cryptprotectUIForbidden  = 0x1
	cryptprotectLocalMachine = 0x4
	enableLineInput          = 0x2
	enableEchoInput          = 0x4
	cfUnicodeText            = 13
)

// entropy ata el blob a este programa: otro uso de DPAPI LocalMachine en la
// PC no lo descifra por accidente.
const entropy = "vetter-agent/secret/v1"

type dataBlob struct {
	size uint32
	data *byte
}

func newBlob(data []byte) dataBlob {
	if len(data) == 0 {
		return dataBlob{}
	}

	return dataBlob{size: uint32(len(data)), data: &data[0]} //nolint:gosec // el secreto entra en uint32
}

func (b *dataBlob) bytes() []byte {
	if b.data == nil {
		return nil
	}

	return append([]byte(nil), unsafe.Slice(b.data, b.size)...) //nolint:gosec // buffer devuelto por DPAPI
}

func protect(plain []byte) ([]byte, error) {
	return dpapi("CryptProtectData", plain)
}

func unprotect(sealed []byte) ([]byte, error) {
	return dpapi("CryptUnprotectData", sealed)
}

func dpapi(procName string, input []byte) ([]byte, error) {
	crypt32 := syscall.NewLazyDLL("crypt32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := crypt32.NewProc(procName)

	in := newBlob(input)
	salt := []byte(entropy)
	extra := newBlob(salt)

	var out dataBlob

	ok, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(&in)),    //nolint:gosec // llamada a la API de Windows
		0,                               // descripción
		uintptr(unsafe.Pointer(&extra)), //nolint:gosec // llamada a la API de Windows
		0, 0,
		cryptprotectUIForbidden|cryptprotectLocalMachine,
		uintptr(unsafe.Pointer(&out)), //nolint:gosec // llamada a la API de Windows
	)
	if ok == 0 {
		return nil, fmt.Errorf("DPAPI %s: %w", procName, callErr)
	}

	defer func() {
		_, _, _ = kernel32.NewProc("LocalFree").Call(uintptr(unsafe.Pointer(out.data))) //nolint:gosec // libera lo que reservó DPAPI
	}()

	return out.bytes(), nil
}

// restrict deja el archivo solo para SYSTEM, Administradores y la cuenta
// actual. Si la tarea corre con otra cuenta, hay que agregarla con icacls.
func restrict(ctx context.Context, path string) error {
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("cuenta actual: %w", err)
	}

	//nolint:gosec // argumentos armados por el programa, sin shell
	cmd := exec.CommandContext(ctx, "icacls", path, "/inheritance:r",
		"/grant:r", "*S-1-5-18:F", "/grant:r", "*S-1-5-32-544:F", "/grant:r", current.Username+":F")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icacls: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// Prompt pide el secreto por la consola mostrando un * por carácter (también
// lo pegado), sin eco del texto. Si stdin no es una consola (un pipe), lee la
// línea tal cual.
func Prompt(label string, stdin *os.File, stderr io.Writer) (string, error) {
	_, _ = fmt.Fprint(stderr, label)

	handle := syscall.Handle(stdin.Fd())

	var mode uint32

	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("leer secreto: %w", err)
		}

		return Clean(line), nil
	}

	setMode := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

	if ok, _, callErr := setMode.Call(uintptr(handle), uintptr(mode&^(enableEchoInput|enableLineInput))); ok == 0 {
		return "", fmt.Errorf("modo de consola: %w", callErr)
	}

	defer func() { _, _, _ = setMode.Call(uintptr(handle), uintptr(mode)) }()

	return ReadMasked(stdin, stderr)
}

// ReadClipboard lee el texto del portapapeles y lo vacía, para que el secreto
// no quede ahí después del pareo.
func ReadClipboard() (string, error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	if ok, _, err := user32.NewProc("OpenClipboard").Call(0); ok == 0 {
		return "", fmt.Errorf("abrir portapapeles: %w", err)
	}

	defer func() { _, _, _ = user32.NewProc("CloseClipboard").Call() }()

	data, _, err := user32.NewProc("GetClipboardData").Call(cfUnicodeText)
	if data == 0 {
		return "", fmt.Errorf("el portapapeles no tiene texto: %w", err)
	}

	ptr, _, err := kernel32.NewProc("GlobalLock").Call(data)
	if ptr == 0 {
		return "", fmt.Errorf("leer portapapeles: %w", err)
	}

	text := utf16At(kernel32, ptr)
	_, _, _ = kernel32.NewProc("GlobalUnlock").Call(data)
	_, _, _ = user32.NewProc("EmptyClipboard").Call()

	return Clean(text), nil
}

// utf16At copia un texto UTF-16 terminado en 0 de memoria de Windows a un
// buffer de Go, con un tope de 4096 caracteres.
func utf16At(kernel32 *syscall.LazyDLL, ptr uintptr) string {
	length, _, _ := kernel32.NewProc("lstrlenW").Call(ptr)
	if length == 0 {
		return ""
	}

	units := make([]uint16, min(length, 4096))

	_, _, _ = kernel32.NewProc("RtlMoveMemory").Call(
		uintptr(unsafe.Pointer(&units[0])), //nolint:gosec // destino: buffer de Go
		ptr,
		uintptr(len(units)*2),
	)

	return syscall.UTF16ToString(units)
}
