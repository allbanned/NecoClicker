//go:build windows

// Package overlay рисует click-ping — прозрачные кружки в точках клика.
//
// Реализация: одно полноэкранное layered click-through topmost окно.
// Клики проходят насквозь, окно не появляется в Alt-Tab и таскбаре.
//
// Производительность: DIB section и memory DC создаются ОДИН РАЗ при Start
// и переиспользуются. Рендер идёт ТОЛЬКО когда есть активные пинги — в
// idle движок overlay ничего не делает (нет flush'а UpdateLayeredWindow).
// Это убирает фоновую нагрузку на GDI/CPU при выключенных кликах.
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

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000

	wsPopup = 0x80000000

	swShowNoActivate = 4

	smCxScreen        = 0
	smCyScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCxVirtualScreen = 78
	smCyVirtualScreen = 79

	hwndTopmost   = ^uintptr(0) // -1
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001

	biRGB        = 0
	dibRgbColors = 0
	ulwAlpha     = 0x00000002
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

type ping struct {
	X, Y    int32
	Started time.Time
}

const (
	pingDuration = 350 * time.Millisecond
	maxRadius    = 36
	pingsCap     = 32
	tickInterval = 33 * time.Millisecond // ~30 fps
)

type Overlay struct {
	hwnd     uintptr
	hScreen  uintptr // device context экрана (кэш)
	hMem     uintptr // memory DC (кэш)
	hBmp     uintptr // DIB section (кэш)
	bits     unsafe.Pointer

	// virtual screen rect
	vx, vy, vw, vh int32

	mu      sync.Mutex
	pings   []ping
	enabled atomic.Bool

	// pixel buffer — указывает на bits, не отдельный slice
	pixels []uint32

	// чтобы знать, нужно ли стирать прошлый кадр
	hadFrame bool

	stopCh    chan struct{}
	stoppedCh chan struct{}
	started   atomic.Bool

	// цвет вспышки (R,G,B) — задаётся снаружи
	r, g, b atomic.Uint32
}

func New() *Overlay {
	o := &Overlay{
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	o.SetColor(255, 35, 80)
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

func (o *Overlay) Ping(x, y int) {
	if !o.enabled.Load() || !o.started.Load() {
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

func (o *Overlay) Start() error {
	if !o.started.CompareAndSwap(false, true) {
		return nil
	}
	started := make(chan error, 1)
	go func() {
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
			close(o.stoppedCh)
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
			close(o.stoppedCh)
			return
		}
		o.hwnd = hwnd

		// КЭШ DIB и memory DC — создаём один раз
		o.hScreen, _, _ = procGetDC.Call(0)
		o.hMem, _, _ = procCreateCompatibleDC.Call(o.hScreen)

		bi := bitmapInfo{}
		bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
		bi.BmiHeader.BiWidth = o.vw
		bi.BmiHeader.BiHeight = -o.vh // top-down
		bi.BmiHeader.BiPlanes = 1
		bi.BmiHeader.BiBitCount = 32
		bi.BmiHeader.BiCompression = biRGB

		var bits unsafe.Pointer
		hBmp, _, _ := procCreateDIBSection.Call(
			o.hMem,
			uintptr(unsafe.Pointer(&bi)),
			uintptr(dibRgbColors),
			uintptr(unsafe.Pointer(&bits)),
			0, 0,
		)
		if hBmp == 0 || bits == nil {
			procDestroyWindow.Call(o.hwnd)
			started <- err
			close(o.stoppedCh)
			return
		}
		o.hBmp = hBmp
		o.bits = bits
		o.pixels = unsafe.Slice((*uint32)(bits), int(o.vw)*int(o.vh))
		procSelectObject.Call(o.hMem, o.hBmp)

		procShowWindow.Call(o.hwnd, swShowNoActivate)
		procSetWindowPos.Call(o.hwnd, hwndTopmost, 0, 0, 0, 0,
			swpNoActivate|swpShowWindow|swpNoMove|swpNoSize)

		started <- nil

		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				o.tick()
			case <-o.stopCh:
				if o.hBmp != 0 {
					procDeleteObject.Call(o.hBmp)
				}
				if o.hMem != 0 {
					procDeleteDC.Call(o.hMem)
				}
				if o.hScreen != 0 {
					procReleaseDC.Call(0, o.hScreen)
				}
				procDestroyWindow.Call(o.hwnd)
				close(o.stoppedCh)
				return
			}
		}
	}()
	return <-started
}

func (o *Overlay) Stop() {
	if !o.started.CompareAndSwap(true, false) {
		return
	}
	close(o.stopCh)
	<-o.stoppedCh
}

// tick — главный цикл рендера. ВАЖНО: если активных пингов нет и в прошлом
// кадре их тоже не было — не делает НИЧЕГО (никаких syscall'ов). Если они
// были но устарели — один последний flush чтобы стереть. Иначе — рендер.
func (o *Overlay) tick() {
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
		if !o.hadFrame {
			return // idle — ничего не делаем
		}
		// стираем последний кадр
		for i := range o.pixels {
			o.pixels[i] = 0
		}
		o.flush()
		o.hadFrame = false
		return
	}

	o.hadFrame = true

	// Рендер: чистим только области под кружки + рисуем
	cr := byte(o.r.Load())
	cg := byte(o.g.Load())
	cb := byte(o.b.Load())

	// проще всего — clear+draw
	for i := range o.pixels {
		o.pixels[i] = 0
	}
	for _, p := range pings {
		t := float64(now.Sub(p.Started)) / float64(pingDuration)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		radius := int32(8 + float64(maxRadius-8)*t)
		alpha := byte(255 * (1 - t))
		o.drawCircleRing(p.X-o.vx, p.Y-o.vy, radius, 3, cr, cg, cb, alpha)
		// центральная точка
		o.drawCircleFilled(p.X-o.vx, p.Y-o.vy, 4, cr, cg, cb, byte(int(alpha)+30))
	}
	o.flush()
}

// drawCircleFilled — закрашенный круг (premultiplied alpha).
func (o *Overlay) drawCircleFilled(cx, cy, r int32, R, G, B, A byte) {
	if r <= 0 || A == 0 {
		return
	}
	pr := uint32(R) * uint32(A) / 255
	pg := uint32(G) * uint32(A) / 255
	pb := uint32(B) * uint32(A) / 255
	pix := (uint32(A) << 24) | (pr << 16) | (pg << 8) | pb

	w, h := o.vw, o.vh
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

// drawCircleRing — кольцо (между r-thickness и r).
func (o *Overlay) drawCircleRing(cx, cy, r, thickness int32, R, G, B, A byte) {
	if r <= 0 || A == 0 {
		return
	}
	pr := uint32(R) * uint32(A) / 255
	pg := uint32(G) * uint32(A) / 255
	pb := uint32(B) * uint32(A) / 255
	pix := (uint32(A) << 24) | (pr << 16) | (pg << 8) | pb

	w, h := o.vw, o.vh
	rOuter2 := r * r
	rInner := r - thickness
	if rInner < 1 {
		rInner = 1
	}
	rInner2 := rInner * rInner

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
			d2 := dx*dx + dy2
			if d2 <= rOuter2 && d2 >= rInner2 {
				o.pixels[row+x] = pix
			}
		}
	}
}

func (o *Overlay) flush() {
	dstPos := point{X: o.vx, Y: o.vy}
	srcPos := point{X: 0, Y: 0}
	sz := size{Cx: o.vw, Cy: o.vh}
	blend := blendFunction{
		BlendOp:             0, // AC_SRC_OVER
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         1, // AC_SRC_ALPHA
	}
	procUpdateLayeredWindow.Call(
		o.hwnd,
		o.hScreen,
		uintptr(unsafe.Pointer(&dstPos)),
		uintptr(unsafe.Pointer(&sz)),
		o.hMem,
		uintptr(unsafe.Pointer(&srcPos)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		uintptr(ulwAlpha),
	)
}
