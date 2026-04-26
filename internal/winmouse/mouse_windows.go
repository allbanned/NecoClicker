//go:build windows

// Package winmouse — точное управление мышью через user32!SendInput.
// Размер структуры INPUT и MOUSEINPUT соответствует ABI Windows на x64
// (40 байт для INPUT с 4-байтовым padding после Type).
package winmouse

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSendInput    = user32.NewProc("SendInput")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

const (
	inputMouse = 0

	mouseeventfMove       = 0x0001
	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfRightDown  = 0x0008
	mouseeventfRightUp    = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp   = 0x0040
	mouseeventfXDown      = 0x0080
	mouseeventfXUp        = 0x0100

	xButton1 = 0x0001 // Mouse4 ("вперёд")
	xButton2 = 0x0002 // Mouse5 ("назад")
)

// MOUSEINPUT — 32 байта на x64.
type mouseInput struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	_pad        uint32 // выравнивание 8-байтового uintptr ниже
	DwExtraInfo uintptr
}

// INPUT с типом MOUSE — 40 байт на x64.
type input struct {
	Type uint32
	_    uint32
	Mi   mouseInput
}

type point struct{ X, Y int32 }

func GetCursor() (int, int) {
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}

func SetCursor(x, y int) {
	procSetCursorPos.Call(uintptr(int32(x)), uintptr(int32(y)))
}

func sendInputs(in []input) {
	if len(in) == 0 {
		return
	}
	procSendInput.Call(
		uintptr(uint32(len(in))),
		uintptr(unsafe.Pointer(&in[0])),
		unsafe.Sizeof(in[0]),
	)
}

// flagsFor возвращает (downFlag, upFlag, mouseData) для конкретной кнопки.
// Для X-кнопок mouseData идентифицирует X1/X2 (для остальных = 0).
func flagsFor(button string) (down, up, data uint32) {
	switch button {
	case "right":
		return mouseeventfRightDown, mouseeventfRightUp, 0
	case "middle":
		return mouseeventfMiddleDown, mouseeventfMiddleUp, 0
	case "x1":
		return mouseeventfXDown, mouseeventfXUp, xButton1
	case "x2":
		return mouseeventfXDown, mouseeventfXUp, xButton2
	default: // left
		return mouseeventfLeftDown, mouseeventfLeftUp, 0
	}
}

// Click имитирует один клик указанной кнопкой. Если useCurrent=false,
// сначала перемещает курсор в (x,y) и кликает там.
// Поддерживаемые имена: "left", "right", "middle", "x1", "x2".
func Click(button string, x, y int, useCurrent bool) {
	if !useCurrent {
		SetCursor(x, y)
	}
	down, up, data := flagsFor(button)
	sendInputs([]input{
		{Type: inputMouse, Mi: mouseInput{DwFlags: down, MouseData: data}},
		{Type: inputMouse, Mi: mouseInput{DwFlags: up, MouseData: data}},
	})
}

// MouseDown — нажать (без отпускания).
func MouseDown(button string) {
	down, _, data := flagsFor(button)
	sendInputs([]input{{Type: inputMouse, Mi: mouseInput{DwFlags: down, MouseData: data}}})
}

// MouseUp — отпустить.
func MouseUp(button string) {
	_, up, data := flagsFor(button)
	sendInputs([]input{{Type: inputMouse, Mi: mouseInput{DwFlags: up, MouseData: data}}})
}
