//go:build windows

package secret

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

// Constantes de user32 que usa la ventana de pareo.
const (
	wsCaption       = 0x00C00000
	wsSysMenu       = 0x00080000
	wsVisible       = 0x10000000
	wsChild         = 0x40000000
	wsTabStop       = 0x00010000
	wsBorder        = 0x00800000
	esPassword      = 0x0020
	esAutoHScroll   = 0x0080
	bsDefPushButton = 0x0001
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmSetFont       = 0x0030
	wmCommand       = 0x0111
	idOK            = 1
	idCancel        = 2
	idEdit          = 100
	colorBtnFace    = 15
	defaultGUIFont  = 17
	idcArrow        = 32512
	mbIconError     = 0x10
	mbIconInfo      = 0x40
	mbTopMost       = 0x40000
	swShow          = 5
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

// win32 junta los procs de user32/kernel32/gdi32 de una ventana.
type win32 struct {
	user32 *syscall.LazyDLL
	gdi32  *syscall.LazyDLL
}

func (w win32) call(name string, args ...uintptr) uintptr {
	dll := w.user32
	if name == "GetStockObject" {
		dll = w.gdi32
	}

	ret, _, _ := dll.NewProc(name).Call(args...)

	return ret
}

func utf16(text string) *uint16 {
	ptr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		empty := uint16(0)
		return &empty
	}

	return ptr
}

// PromptWindow abre una ventana con un campo de contraseña (se pega con
// Ctrl+V o click derecho) y devuelve lo ingresado. Cancelar o cerrar la
// ventana devuelve error.
func PromptWindow() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	api := win32{user32: syscall.NewLazyDLL("user32.dll"), gdi32: syscall.NewLazyDLL("gdi32.dll")}
	instance, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)

	var (
		edit     uintptr
		value    string
		accepted bool
	)

	proc := func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case wmCommand:
			switch wParam & 0xffff {
			case idOK:
				value, accepted = windowText(api, edit), true
				api.call("DestroyWindow", hwnd)
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

	className := utf16("VetterAgentPair")
	class := wndClassEx{
		wndProc:    syscall.NewCallback(proc),
		instance:   instance,
		cursor:     api.call("LoadCursorW", 0, idcArrow),
		background: colorBtnFace + 1,
		className:  className,
	}
	class.size = uint32(unsafe.Sizeof(class))

	if api.call("RegisterClassExW", uintptr(unsafe.Pointer(&class))) == 0 { //nolint:gosec // API de Windows
		return "", errors.New("no se pudo registrar la ventana")
	}

	hwnd, controls := buildWindow(api, instance, className)
	if hwnd == 0 {
		return "", errors.New("no se pudo crear la ventana")
	}

	edit = controls

	api.call("ShowWindow", hwnd, swShow)
	api.call("SetForegroundWindow", hwnd)
	api.call("SetFocus", edit)
	loop(api, hwnd)

	if !accepted {
		return "", errors.New("pareo cancelado")
	}

	return Clean(value), nil
}

// buildWindow crea la ventana centrada con etiqueta, campo y botones.
// Devuelve la ventana y el campo de texto.
func buildWindow(api win32, instance uintptr, className *uint16) (uintptr, uintptr) {
	const width, height = 480, 170

	x := (int(api.call("GetSystemMetrics", 0)) - width) / 2
	y := (int(api.call("GetSystemMetrics", 1)) - height) / 2

	hwnd := api.call("CreateWindowExW", 0,
		uintptr(unsafe.Pointer(className)),                     //nolint:gosec // API de Windows
		uintptr(unsafe.Pointer(utf16("vetter-agent — pareo"))), //nolint:gosec // API de Windows
		wsCaption|wsSysMenu, uintptr(x), uintptr(y), width, height, 0, 0, instance, 0)
	if hwnd == 0 {
		return 0, 0
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

	child("STATIC", "Pegá el secreto de instalación (vgs_…) y apretá Parear:", 0, 16, 14, 440, 20, 0)
	edit := child("EDIT", "", wsBorder|wsTabStop|esPassword|esAutoHScroll, 16, 40, 440, 24, idEdit)
	child("BUTTON", "Parear", wsTabStop|bsDefPushButton, 266, 82, 90, 28, idOK)
	child("BUTTON", "Cancelar", wsTabStop, 366, 82, 90, 28, idCancel)

	return hwnd, edit
}

func loop(api win32, hwnd uintptr) {
	var message msg

	for {
		ret := api.call("GetMessageW", uintptr(unsafe.Pointer(&message)), 0, 0, 0) //nolint:gosec // API de Windows
		if ret == 0 || int32(ret) == -1 {                                          //nolint:gosec // BOOL de Windows
			return
		}

		// Tab entre controles y Enter = Parear.
		if api.call("IsDialogMessageW", hwnd, uintptr(unsafe.Pointer(&message))) != 0 { //nolint:gosec // API de Windows
			continue
		}

		api.call("TranslateMessage", uintptr(unsafe.Pointer(&message))) //nolint:gosec // API de Windows
		api.call("DispatchMessageW", uintptr(unsafe.Pointer(&message))) //nolint:gosec // API de Windows
	}
}

func windowText(api win32, hwnd uintptr) string {
	buf := make([]uint16, 1024)
	n := api.call("GetWindowTextW", hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))) //nolint:gosec // API de Windows

	return syscall.UTF16ToString(buf[:n])
}

// Notify muestra el resultado en un cartel, para cuando `pair --gui` se lanza
// sin consola visible (Win+R).
func Notify(text string, failed bool) {
	icon := uintptr(mbIconInfo)
	if failed {
		icon = mbIconError
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	_, _, _ = user32.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(utf16(text))),                   //nolint:gosec // API de Windows
		uintptr(unsafe.Pointer(utf16("vetter-agent — pareo"))), //nolint:gosec // API de Windows
		icon|mbTopMost)
}
