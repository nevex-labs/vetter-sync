//go:build windows

package secret

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

// Constantes propias de la ventana de instalación.
const (
	idPaste  = 101
	idUpdate = 103
	idTask   = 104
	ssLeft   = 0x0000
)

// RunSetup abre la ventana de instalación y no vuelve hasta que se cierra.
// Los botones llaman a las acciones; la ventana solo muestra el texto que
// devuelven.
func RunSetup(actions SetupActions) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	api := win32{user32: syscall.NewLazyDLL("user32.dll"), gdi32: syscall.NewLazyDLL("gdi32.dll")}
	instance, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)

	var (
		edit, status uintptr
		window       uintptr
	)

	show := func(text string) {
		if status != 0 && text != "" {
			api.call("SetWindowTextW", status, uintptr(unsafe.Pointer(utf16(text)))) //nolint:gosec // API de Windows
			api.call("InvalidateRect", window, 0, 1)
			api.call("UpdateWindow", window)
		}
	}

	proc := func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case wmCommand:
			switch wParam & 0xffff {
			case idPaste:
				token, err := ReadClipboard()
				if err != nil {
					show("No se pudo leer el portapapeles: " + err.Error())

					return 0
				}

				api.call("SetWindowTextW", edit, uintptr(unsafe.Pointer(utf16(Clean(token))))) //nolint:gosec // API de Windows
				show("Secreto pegado. Revisá que termine en los caracteres que ves en el panel.")
			case idOK:
				token := Clean(windowText(api, edit))

				if token == "" {
					show("Pegá primero el secreto de instalación.")

					return 0
				}

				show("Validando con el servidor…")
				show(actions.Pair(token))
			case idUpdate:
				show("Buscando y bajando el último release…")
				show(actions.Update())
			case idTask:
				show("Abriendo el instalador de la tarea…")
				show(actions.Install())
			case idCancel:
				api.call("DestroyWindow", hwnd)
			}

			return 0
		case wmClose:
			api.call("DestroyWindow", hwnd)
			return 0
		case wmDestroy:
			api.call("PostQuitMessage", 0)
			return 0
		}

		return api.call("DefWindowProcW", hwnd, uintptr(message), wParam, lParam)
	}

	className := utf16("VetterAgentSetup")
	class := wndClassEx{
		wndProc:    syscall.NewCallback(proc),
		instance:   instance,
		cursor:     api.call("LoadCursorW", 0, idcArrow),
		background: colorBtnFace + 1,
		className:  className,
	}
	class.size = uint32(unsafe.Sizeof(class))

	if api.call("RegisterClassExW", uintptr(unsafe.Pointer(&class))) == 0 { //nolint:gosec // API de Windows
		return errors.New("no se pudo registrar la ventana")
	}

	hwnd, controls := buildSetupWindow(api, instance, className)
	if hwnd == 0 {
		return errors.New("no se pudo crear la ventana")
	}

	edit, status, window = controls[0], controls[1], hwnd

	show(actions.Status())

	api.call("ShowWindow", hwnd, swShow)
	api.call("SetForegroundWindow", hwnd)
	api.call("SetFocus", edit)
	loop(api, hwnd)

	return nil
}

func buildSetupWindow(api win32, instance uintptr, className *uint16) (uintptr, []uintptr) {
	const width, height = 600, 330

	x := (int(api.call("GetSystemMetrics", 0)) - width) / 2
	y := (int(api.call("GetSystemMetrics", 1)) - height) / 2

	hwnd := api.call("CreateWindowExW", 0,
		uintptr(unsafe.Pointer(className)),                              //nolint:gosec // API de Windows
		uintptr(unsafe.Pointer(utf16("Vetter — instalación de la PC"))), //nolint:gosec // API de Windows
		wsCaption|wsSysMenu, uintptr(x), uintptr(y), width, height, 0, 0, instance, 0)
	if hwnd == 0 {
		return 0, nil
	}

	font := api.call("GetStockObject", defaultGUIFont)
	child := func(class, text string, style, left, top, w, h, id uintptr) uintptr {
		ctrl := api.call("CreateWindowExW", 0,
			uintptr(unsafe.Pointer(utf16(class))), //nolint:gosec // API de Windows
			uintptr(unsafe.Pointer(utf16(text))),  //nolint:gosec // API de Windows
			wsChild|wsVisible|style, left, top, w, h, hwnd, id, instance, 0)
		api.call("SendMessageW", ctrl, wmSetFont, font, 1)

		return ctrl
	}

	child("STATIC", "Secreto de instalación (vgs_…):", 0, 16, 14, 400, 20, 0)
	edit := child("EDIT", "", wsBorder|wsTabStop|esPassword|esAutoHScroll, 16, 38, 460, 26, idEdit)
	child("BUTTON", "Pegar", wsTabStop, 486, 38, 90, 28, idPaste)
	child("BUTTON", "Guardar pareo", wsTabStop|bsDefPushButton, 16, 78, 150, 30, idOK)
	child("BUTTON", "Actualizar agente", wsTabStop, 176, 78, 150, 30, idUpdate)
	child("BUTTON", "Instalar tarea", wsTabStop, 336, 78, 150, 30, idTask)
	child("BUTTON", "Cerrar", wsTabStop, 496, 78, 80, 30, idCancel)

	status := child("STATIC", "", ssLeft|wsBorder, 16, 124, 560, 160, 0)

	return hwnd, []uintptr{edit, status}
}
