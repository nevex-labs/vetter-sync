//go:build !windows

package secret

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// Fuera de Windows no hay DPAPI: compila para el CI, pero no guarda nada.
var errNoDPAPI = errors.New("DPAPI solo existe en Windows")

func protect([]byte) ([]byte, error) { return nil, errNoDPAPI }

func unprotect([]byte) ([]byte, error) { return nil, errNoDPAPI }

func restrict(context.Context, string) error { return nil }

// PromptWindow no existe fuera de Windows.
func PromptWindow() (string, error) { return "", errors.New("ventana solo en Windows") }

// Notify no hace nada fuera de Windows.
func Notify(string, bool) {}

// ReadClipboard no existe fuera de Windows.
func ReadClipboard() (string, error) { return "", errors.New("portapapeles solo en Windows") }

// Prompt lee la línea tal cual: fuera de Windows no se apaga el eco.
func Prompt(label string, stdin *os.File, stderr io.Writer) (string, error) {
	_, _ = fmt.Fprint(stderr, label)

	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("leer secreto: %w", err)
	}

	return Clean(line), nil
}
