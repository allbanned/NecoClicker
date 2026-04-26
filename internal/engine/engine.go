// Package engine выполняет кликер и цепочки в фоновых goroutine'ах
// с безопасной отменой через context, опциональными лимитами,
// jitter'ом, паузой и опциональным click-ping overlay'ем.
package engine

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"NecoClicker/internal/macro"
	"NecoClicker/internal/winmouse"
)

type Logger func(string)

// Pinger — наблюдатель кликов (для overlay).
type Pinger interface {
	Ping(x, y int)
}

type CPSReport struct {
	CPS   float64 `json:"cps"`
	Total uint64  `json:"total"`
}

type CPSCallback func(CPSReport)

type Engine struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	dryRun    bool
	log       Logger
	listeners []func(running bool)

	clickCount atomic.Uint64
	cpsCancel  context.CancelFunc

	// pause/resume — атомарный флаг
	paused atomic.Bool

	// глобальное смещение клика ±N px (применяется ко всем источникам)
	clickJitterPx atomic.Int32

	// pinger опционален; если nil — overlay не используется
	pinger Pinger

	rng     *rand.Rand
	rngOnce sync.Once
}

func New(log Logger) *Engine {
	if log == nil {
		log = func(string) {}
	}
	return &Engine{log: log}
}

// SetPinger — назначить overlay-callback на каждый клик.
func (e *Engine) SetPinger(p Pinger) {
	e.mu.Lock()
	e.pinger = p
	e.mu.Unlock()
}

// SetClickJitterPx — глобальный random offset ±N px.
func (e *Engine) SetClickJitterPx(n int) {
	if n < 0 {
		n = 0
	}
	e.clickJitterPx.Store(int32(n))
}

func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Engine) IsDryRun() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dryRun
}

func (e *Engine) SetDryRun(v bool) {
	e.mu.Lock()
	e.dryRun = v
	e.mu.Unlock()
}

// ---- pause / resume --------------------------------------------------------

func (e *Engine) IsPaused() bool { return e.paused.Load() }

// Pause замораживает движок: следующий fire не выполнится пока не Resume.
func (e *Engine) Pause() {
	if !e.IsRunning() {
		return
	}
	if e.paused.CompareAndSwap(false, true) {
		e.log("Paused")
	}
}

func (e *Engine) Resume() {
	if e.paused.CompareAndSwap(true, false) {
		e.log("Resumed")
	}
}

// TogglePause — для глобального хоткея пауза/продолжить.
func (e *Engine) TogglePause() {
	if e.paused.Load() {
		e.Resume()
	} else {
		e.Pause()
	}
}

// waitWhilePaused блокирует пока есть пауза, прерываясь на ctx.Done.
func (e *Engine) waitWhilePaused(ctx context.Context) bool {
	for e.paused.Load() {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
	return ctx.Err() == nil
}

func (e *Engine) OnStateChange(fn func(bool)) {
	e.mu.Lock()
	e.listeners = append(e.listeners, fn)
	e.mu.Unlock()
}

func (e *Engine) emit(running bool) {
	e.mu.Lock()
	e.running = running
	if !running {
		// при остановке всегда выходим из паузы
		e.paused.Store(false)
	}
	ls := append([]func(bool){}, e.listeners...)
	e.mu.Unlock()
	for _, fn := range ls {
		fn(running)
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	c := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	if c != nil {
		c()
	}
	e.paused.Store(false)
	e.wg.Wait()
}

func (e *Engine) Toggle(start func()) {
	if e.IsRunning() {
		e.Stop()
		return
	}
	start()
}

func (e *Engine) start() context.Context {
	e.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()
	e.emit(true)
	e.wg.Add(1)
	return ctx
}

func (e *Engine) finish() {
	e.wg.Done()
	e.emit(false)
}

// ---- click counting ---------------------------------------------------------

func (e *Engine) ResetClicks()        { e.clickCount.Store(0) }
func (e *Engine) TotalClicks() uint64 { return e.clickCount.Load() }

func (e *Engine) StartCPSReporter(parent context.Context, cb CPSCallback) {
	ctx, cancel := context.WithCancel(parent)
	e.mu.Lock()
	if e.cpsCancel != nil {
		e.cpsCancel()
	}
	e.cpsCancel = cancel
	e.mu.Unlock()

	go func() {
		const tick = 250 * time.Millisecond
		const window = 4
		t := time.NewTicker(tick)
		defer t.Stop()

		samples := make([]uint64, 0, window)
		last := e.clickCount.Load()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cur := e.clickCount.Load()
				delta := cur - last
				last = cur
				samples = append(samples, delta)
				if len(samples) > window {
					samples = samples[len(samples)-window:]
				}
				var sum uint64
				for _, s := range samples {
					sum += s
				}
				secs := float64(len(samples)) * tick.Seconds()
				if secs <= 0 {
					secs = tick.Seconds()
				}
				cb(CPSReport{
					CPS:   float64(sum) / secs,
					Total: cur,
				})
			}
		}
	}()
}

func (e *Engine) StopCPSReporter() {
	e.mu.Lock()
	c := e.cpsCancel
	e.cpsCancel = nil
	e.mu.Unlock()
	if c != nil {
		c()
	}
}

// ---- helpers ----------------------------------------------------------------

func (e *Engine) ensureRng() {
	e.rngOnce.Do(func() {
		e.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
}

// jitterPos возвращает (x,y) с добавленным глобальным jitter'ом ±N px.
func (e *Engine) jitterPos(x, y int) (int, int) {
	n := int(e.clickJitterPx.Load())
	if n <= 0 {
		return x, y
	}
	e.ensureRng()
	return x + e.rng.Intn(2*n+1) - n, y + e.rng.Intn(2*n+1) - n
}

// doClick — единая точка отправки клика: jitter, инкремент, overlay-ping
// и (в реальном режиме) реальный SendInput.
func (e *Engine) doClick(button string, x, y int, useCurrent bool) {
	e.clickCount.Add(1)

	// если useCurrent — берём текущую позицию для overlay (и jitter не применяем)
	var px, py int
	if useCurrent {
		px, py = winmouse.GetCursor()
	} else {
		px, py = e.jitterPos(x, y)
	}

	if !e.IsDryRun() {
		if useCurrent {
			winmouse.Click(button, 0, 0, true)
		} else {
			winmouse.Click(button, px, py, false)
		}
	}

	e.mu.Lock()
	p := e.pinger
	e.mu.Unlock()
	if p != nil {
		p.Ping(px, py)
	}
}

// ---- runners ----------------------------------------------------------------

func (e *Engine) RunSimple(cfg macro.SimpleConfig) {
	e.RunSimpleLimited(cfg, macro.RunLimits{})
}

func (e *Engine) RunSimpleLimited(cfg macro.SimpleConfig, lim macro.RunLimits) {
	ctx := e.start()
	go func() {
		defer e.finish()
		btn := string(cfg.Button)
		if btn == "" {
			btn = "left"
		}
		ms := cfg.IntervalMs
		if ms < 0 {
			ms = 0
		}
		desc := fmt.Sprintf("Simple started: btn=%s interval=%gms", btn, ms)
		if lim.JitterMs > 0 {
			desc += fmt.Sprintf(" jitter=±%gms", lim.JitterMs/2)
		}
		if lim.DurationSec > 0 {
			desc += fmt.Sprintf(" duration=%ds", lim.DurationSec)
		}
		if lim.MaxClicks > 0 {
			desc += fmt.Sprintf(" max=%d", lim.MaxClicks)
		}
		e.log(desc)

		var deadlineCancel context.CancelFunc
		if lim.DurationSec > 0 {
			ctx, deadlineCancel = context.WithTimeout(ctx, time.Duration(lim.DurationSec)*time.Second)
			defer deadlineCancel()
		}

		startTotal := e.clickCount.Load()
		e.ensureRng()

		fire := func() {
			if e.IsDryRun() {
				x, y := winmouse.GetCursor()
				if !cfg.UseCurrent {
					x, y = cfg.X, cfg.Y
				}
				e.log(fmt.Sprintf("[dry] click %s at (%d,%d)", btn, x, y))
			}
			e.doClick(btn, cfg.X, cfg.Y, cfg.UseCurrent)
		}

		clicksDone := func() bool {
			if lim.MaxClicks == 0 {
				return false
			}
			return e.clickCount.Load()-startTotal >= lim.MaxClicks
		}

		nextDelay := func() time.Duration {
			d := ms
			if lim.JitterMs > 0 {
				d += (e.rng.Float64() - 0.5) * lim.JitterMs
				if d < 0 {
					d = 0
				}
			}
			return time.Duration(d * float64(time.Millisecond))
		}

		// Hot path: ms == 0 без джиттера → tight loop
		if ms == 0 && lim.JitterMs == 0 {
			for {
				if ctx.Err() != nil || clicksDone() {
					e.log("Simple stopped")
					return
				}
				if !e.waitWhilePaused(ctx) {
					return
				}
				fire()
			}
		}

		if !e.waitWhilePaused(ctx) {
			return
		}
		fire()
		if clicksDone() {
			e.log("Simple stopped (max clicks)")
			return
		}
		for {
			d := nextDelay()
			select {
			case <-ctx.Done():
				e.log("Simple stopped")
				return
			case <-time.After(d):
				if !e.waitWhilePaused(ctx) {
					return
				}
				fire()
				if clicksDone() {
					e.log("Simple stopped (max clicks)")
					return
				}
			}
		}
	}()
}

// RunSequence — пошаговый кликер: проходит по seq.Steps, кликая в каждой
// точке с задержкой seq.IntervalMs между шагами. seq.Loops=0 → бесконечно.
func (e *Engine) RunSequence(seq macro.Sequence) {
	if len(seq.Steps) == 0 {
		return
	}
	ctx := e.start()
	go func() {
		defer e.finish()
		e.log(fmt.Sprintf("Sequence started: %d steps, interval=%gms, loops=%d",
			len(seq.Steps), seq.IntervalMs, seq.Loops))

		ms := seq.IntervalMs
		if ms < 0 {
			ms = 0
		}
		d := time.Duration(ms * float64(time.Millisecond))

		infinite := seq.Loops <= 0
		for i := 0; infinite || i < seq.Loops; i++ {
			for idx, s := range seq.Steps {
				if ctx.Err() != nil {
					e.log("Sequence stopped")
					return
				}
				if !e.waitWhilePaused(ctx) {
					return
				}
				btn := string(s.Button)
				if btn == "" {
					btn = "left"
				}
				if e.IsDryRun() {
					e.log(fmt.Sprintf("[%d][dry] step %s (%d,%d)", idx+1, btn, s.X, s.Y))
				} else {
					e.log(fmt.Sprintf("[%d] step %s (%d,%d)", idx+1, btn, s.X, s.Y))
				}
				e.doClick(btn, s.X, s.Y, false)

				// задержка после шага (последний шаг последней итерации можно не ждать)
				lastStep := idx == len(seq.Steps)-1
				lastLoop := !infinite && i == seq.Loops-1
				if lastStep && lastLoop {
					break
				}
				if d > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(d):
					}
				}
			}
		}
		e.log("Sequence done")
	}()
}

func (e *Engine) RunChain(chain macro.Chain) {
	ctx := e.start()
	go func() {
		defer e.finish()
		e.log(fmt.Sprintf("Chain %q started (loops=%d, steps=%d)", chain.Name, chain.Loops, len(chain.Actions)))
		infinite := chain.Loops <= 0
		for i := 0; infinite || i < chain.Loops; i++ {
			for idx, a := range chain.Actions {
				if ctx.Err() != nil {
					e.log("Chain stopped")
					return
				}
				if !e.waitWhilePaused(ctx) {
					return
				}
				if !e.executeAction(ctx, a, idx) {
					return
				}
			}
		}
		e.log(fmt.Sprintf("Chain %q done", chain.Name))
	}()
}

func (e *Engine) executeAction(ctx context.Context, a macro.Action, idx int) bool {
	dry := e.IsDryRun()

	switch a.Type {
	case macro.ActionClick:
		x, y := a.X, a.Y
		if a.Relative {
			cx, cy := winmouse.GetCursor()
			x, y = cx+a.X, cy+a.Y
		} else if a.UseCurrent {
			x, y = winmouse.GetCursor()
		}
		btn := string(a.Button)
		if btn == "" {
			btn = "left"
		}
		if dry {
			e.log(fmt.Sprintf("[%d][dry] click %s (%d,%d)", idx, btn, x, y))
		} else {
			e.log(fmt.Sprintf("[%d] click %s (%d,%d)", idx, btn, x, y))
		}
		e.doClick(btn, x, y, a.UseCurrent && !a.Relative)

	case macro.ActionMove:
		x, y := a.X, a.Y
		if a.Relative {
			cx, cy := winmouse.GetCursor()
			x, y = cx+a.X, cy+a.Y
		}
		if dry {
			e.log(fmt.Sprintf("[%d][dry] move (%d,%d)", idx, x, y))
		} else {
			winmouse.SetCursor(x, y)
			e.log(fmt.Sprintf("[%d] move (%d,%d)", idx, x, y))
		}

	case macro.ActionDrag:
		btn := string(a.Button)
		if btn == "" {
			btn = "left"
		}
		startX, startY := a.X, a.Y
		endX, endY := a.EndX, a.EndY
		if a.Relative {
			cx, cy := winmouse.GetCursor()
			startX, startY = cx+a.X, cy+a.Y
			endX, endY = cx+a.EndX, cy+a.EndY
		}
		dur := time.Duration(a.DurationMs) * time.Millisecond
		if dry {
			e.log(fmt.Sprintf("[%d][dry] drag %s (%d,%d)→(%d,%d) %v", idx, btn, startX, startY, endX, endY, dur))
		} else {
			e.log(fmt.Sprintf("[%d] drag %s (%d,%d)→(%d,%d) %v", idx, btn, startX, startY, endX, endY, dur))
			winmouse.SetCursor(startX, startY)
			winmouse.MouseDown(btn)
			// плавное движение в N шагов
			steps := int(dur / (10 * time.Millisecond))
			if steps < 1 {
				steps = 1
			}
			for i := 1; i <= steps; i++ {
				if ctx.Err() != nil {
					winmouse.MouseUp(btn)
					return false
				}
				t := float64(i) / float64(steps)
				x := startX + int(float64(endX-startX)*t)
				y := startY + int(float64(endY-startY)*t)
				winmouse.SetCursor(x, y)
				if steps > 1 {
					time.Sleep(dur / time.Duration(steps))
				}
			}
			winmouse.MouseUp(btn)
			e.mu.Lock()
			p := e.pinger
			e.mu.Unlock()
			if p != nil {
				p.Ping(endX, endY)
			}
		}
		e.clickCount.Add(1)

	case macro.ActionDelay:
		d := time.Duration(a.DelayMs) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
		}
	}
	return true
}
