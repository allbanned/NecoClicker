//go:build windows

// Package overlay рисует click-ping — прозрачные кружки в точках клика.
// Реализовано как одно полноэкранное layered-окно WS_EX_LAYERED |
// WS_EX_TRANSPARENT | WS_EX_TOPMOST | WS_EX_TOOLWINDOW. Клики проходят
// насквозь, окно не появляется в Alt-Tab и таскбаре. Перерисовывается
// по таймеру пока есть активные пинги.
package overlay

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW          = user32.NewProc("RegisterClassExW")
	procCreateWindowExW           = user32.NewProc("CreateWindowExW")
	procDestroyWindow             = user32.NewProc("DestroyWindow")
	procShowWindow                = user32.NewProc("ShowWindow")
	procUpdateLayeredWindow       = user32.NewProc("UpdateLayeredWindow")
	procGetMessageW               = user32.NewProc("GetMessageW")
	procPostThreadMessageW        = user32.NewProc("PostThreadMessageW")
	procDefWindowProcW            = user32.NewProc("DefWindowProcW")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
	procSetWindowPos              = user32.NewProc("SetWindowPos")
	procInvalidateRect            = user32.NewProc("InvalidateRect")
	procGetDC                     = user32.NewProc("GetDC")
	procReleaseDC                 = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC        = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                  = gdi32.NewProc("DeleteDC")
	procCreateDIBSection          = gdi32.NewProc("CreateDIBSection")
	procSelectObject              = gdi32.NewProc("SelectObject")
	procDeleteObject              = gdi32.NewProc("DeleteObject")
	procGetCurrentThreadId        = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW          = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000

	wsPopup = 0x80000000

	swShowNoActivate = 4
	swHide           = 0

	smCxScreen        = 0
	smCyScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCxVirtualScreen = 78
	smCyVirtualScreen = 79

	wmQuit  = 0x0012
	wmTimer = 0x0113

	hwndTopmost  = ^uintptr(0) // -1
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	// CreateDIBSection
	biRGB         = 0
	dibRgbColors  = 0
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type size struct{ Cx, Cy int32 }

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

type winMsg struct {
	Hwnd     uintptr
	Message  uint32
	_        uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	X, Y     int32
	LPrivate uint32
}

type ping struct {
	X, Y    int32
	Started time.Time
}

const (
	pingDuration = 350 * time.Millisecond
	maxRadius    = 36 // максимальный радиус кружка
	pingsCap     = 32
)

type Overlay struct {
	hwnd     uintptr
	threadID uint32
	stopped  chan struct{}

	// virtual screen rect (multi-monitor)
	vx, vy, vw, vh int32

	mu      sync.Mutex
	pings   []ping
	enabled atomic.Bool

	// двойной буфер: ARGB пиксели для UpdateLayeredWindow
	pixels []uint32

	// первичный цвет (HSL primary темы) — задаётся снаружи
	r, g, b atomic.Uint32 // 0..255 packed in low bits
}

func New() *Overlay {
	o := &Overlay{stopped: make(chan struct{})}
	o.SetColor(255, 35, 80) // дефолт — алый/розовый
	o.enabled.Store(true)
	return o
}

func (o *Overlay) Enable(v bool) { o.enabled.Store(v) }
func (o *Overlay) IsEnabled() bool { return o.enabled.Load() }

func (o *Overlay) SetColor(r, g, b byte) {
	o.r.Store(uint32(r))
	o.g.Store(uint32(g))
	o.b.Store(uint32(b))
}

// Ping — добавить вспышку в (X,Y) экранных координатах.
func (o *Overlay) Ping(x, y int) {
	if !o.enabled.Load() {
		return
	}
	o.mu.Lock()
	if len(o.pings) >= pingsCap {
		o.pings = o.pings[1:]
	}
	o.pings = append(o.pings, ping{X: int32(x), Y: int32(y), Started: time.Now()})
	o.mu.Unlock()
}

func getSysMetric(idx uintptr) int32 {
	r, _, _ := procGetSystemMetrics.Call(idx)
	return int32(r)
}

// Start запускает overlay-окно в отдельной OS-thread.
func (o *Overlay) Start() error {
	started := make(chan error, 1)
	go func() {
		// thread будет жить до Stop — без UnlockOSThread
		// runtime.LockOSThread() // не обязательно для overlay; добавим если будут проблемы

		tid, _, _ := procGetCurrentThreadId.Call()
		o.threadID = uint32(tid)

		hInst, _, _ := procGetModuleHandleW.Call(0)

		className, _ := syscall.UTF16PtrFromString("NecoClickerOverlay")
		windowTitle, _ := syscall.UTF16PtrFromString("")

		wndProcCb := syscall.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
			r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		})

		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   wndProcCb,
			hInstance:     hInst,
			lpszClassName: className,
		}
		atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			started <- err
			close(o.stopped)
			return
		}

		o.vx = getSysMetric(smXVirtualScreen)
		o.vy = getSysMetric(smYVirtualScreen)
		o.vw = getSysMetric(smCxVirtualScreen)
		o.vh = getSysMetric(smCyVirtualScreen)
		if o.vw <= 0 {
			o.vw = getSysMetric(smCxScreen)
		}
		if o.vh <= 0 {
			o.vh = getSysMetric(smCyScreen)
		}

		hwnd, _, err := procCreateWindowExW.Call(
			uintptr(wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate),
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windowTitle)),
			uintptr(wsPopup),
			uintptr(int32(o.vx)),
			uintptr(int32(o.vy)),
			uintptr(int32(o.vw)),
			uintptr(int32(o.vh)),
			0, 0, hInst, 0,
		)
		if hwnd == 0 {
			started <- err
			close(o.stopped)
			return
		}
		o.hwnd = hwnd

		o.pixels = make([]uint32, int(o.vw)*int(o.vh))

		procShowWindow.Call(o.hwnd, swShowNoActivate)
		// гарантируем topmost
		procSetWindowPos.Call(o.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoActivate|swpShowWindow|0x0001|0x0002) // NOMOVE|NOSIZE

		started <- nil

		// рендер-цикл: 60fps пока есть пинги
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if !o.tick() {
					// нет пингов — продолжаем висеть, ждём
				}
			case <-o.stopChannel():
				procDestroyWindow.Call(o.hwnd)
				close(o.stopped)
				return
			}
		}
	}()
	return <-started
}

// stopChannel возвращает канал, который будет закрыт при Stop().
// Реализован через nil-канал + flag. Упрощённо: используем quitCh.
var quitCh = make(chan struct{})

func (o *Overlay) stopChannel() <-chan struct{} { return quitCh }

func (o *Overlay) Stop() {
	select {
	case <-quitCh:
	default:
		close(quitCh)
	}
	<-o.stopped
}

// tick фильтрует устаревшие пинги, рендерит активные, возвращает true
// если пинги были.
func (o *Overlay) tick() bool {
	o.mu.Lock()
	now := time.Now()
	alive := o.pings[:0]
	for _, p := range o.pings {
		if now.Sub(p.Started) < pingDuration {
			alive = append(alive, p)
		}
	}
	o.pings = alive
	pings := append([]ping(nil), o.pings...)
	o.mu.Unlock()

	if len(pings) == 0 {
		// если в прошлый кадр что-то рисовали — стереть
		for i := range o.pixels {
			o.pixels[i] = 0
		}
		o.flush()
		return false
	}

	// очищаем буфер
	for i := range o.pixels {
		o.pixels[i] = 0
	}

	cr := byte(o.r.Load())
	cg := byte(o.g.Load())
	cb := byte(o.b.Load())

	for _, p := range pings {
		t := float64(now.Sub(p.Started)) / float64(pingDuration) // 0..1
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		// радиус растёт линейно; alpha убывает
		radius := int32(float64(maxRadius) * t)
		alpha := byte(255 * (1 - t))
		o.drawCircle(p.X-o.vx, p.Y-o.vy, radius, cr, cg, cb, alpha)
	}

	o.flush()
	return true
}

// drawCircle — кольцо (анти-rastergraphics): рисуем заполненный круг с
// premultiplied-alpha (требование UpdateLayeredWindow при ULW_ALPHA).
func (o *Overlay) drawCircle(cx, cy, r int32, R, G, B, A byte) {
	if r <= 0 || A == 0 {
		return
	}
	// premultiplied
	pr := uint32(R) * uint32(A) / 255
	pg := uint32(G) * uint32(A) / 255
	pb := uint32(B) * uint32(A) / 255
	pix := (uint32(A) << 24) | (pr << 16) | (pg << 8) | pb

	w := o.vw
	h := o.vh
	r2 := r * r
	x0 := cx - r
	if x0 < 0 {
		x0 = 0
	}
	y0 := cy - r
	if y0 < 0 {
		y0 = 0
	}
	x1 := cx + r
	if x1 >= w {
		x1 = w - 1
	}
	y1 := cy + r
	if y1 >= h {
		y1 = h - 1
	}
	for y := y0; y <= y1; y++ {
		dy := y - cy
		dy2 := dy * dy
		row := y * w
		for x := x0; x <= x1; x++ {
			dx := x - cx
			if dx*dx+dy2 <= r2 {
				o.pixels[row+x] = pix
			}
		}
	}
}

// flush обновляет layered-окно содержимым o.pixels через UpdateLayeredWindow.
func (o *Overlay) flush() {
	hScreen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hScreen)

	hMem, _, _ := procCreateCompatibleDC.Call(hScreen)
	defer procDeleteDC.Call(hMem)

	bi := bitmapInfo{}
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = o.vw
	bi.BmiHeader.BiHeight = -o.vh // top-down
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = biRGB

	var bits unsafe.Pointer
	hBmp, _, _ := procCreateDIBSection.Call(
		hMem,
		uintptr(unsafe.Pointer(&bi)),
		uintptr(dibRgbColors),
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hBmp == 0 || bits == nil {
		return
	}
	defer procDeleteObject.Call(hBmp)

	// копируем наш буфер в DIB
	dst := unsafe.Slice((*uint32)(bits), len(o.pixels))
	copy(dst, o.pixels)

	old, _, _ := procSelectObject.Call(hMem, hBmp)
	defer procSelectObject.Call(hMem, old)

	dstPos := point{X: o.vx, Y: o.vy}
	srcPos := point{X: 0, Y: 0}
	sz := size{Cx: o.vw, Cy: o.vh}
	blend := blendFunction{
		BlendOp:             0, // AC_SRC_OVER
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         1, // AC_SRC_ALPHA
	}
	const ulwAlpha = 0x00000002
	procUpdateLayeredWindow.Call(
		o.hwnd,
		hScreen,
		uintptr(unsafe.Pointer(&dstPos)),
		uintptr(unsafe.Pointer(&sz)),
		hMem,
		uintptr(unsafe.Pointer(&srcPos)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		uintptr(ulwAlpha),
	)
}
