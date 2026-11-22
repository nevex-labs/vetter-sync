//go:build !windows

package secret

import "errors"

// RunSetup no existe fuera de Windows: la instalación es una ventana de Win32.
func RunSetup(SetupActions) error {
	return errors.New("la ventana de instalación es solo para Windows")
}
