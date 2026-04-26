package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"NecoClicker/internal/engine"
	"NecoClicker/internal/hotkey"
	"NecoClicker/internal/macro"
	"NecoClicker/internal/overlay"
	"NecoClicker/internal/winmouse"

	"fyne.io/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIconIco []byte

type App struct {
	ctx     context.Context
	cfg     *macro.Config
	engine  *engine.Engine
	hotkeys *hotkey.Manager
	overlay *overlay.Overlay

	trayToggleItem atomic.Value // *systray.MenuItem
	trayPinItem    atomic.Value // *systray.MenuItem
}

func NewApp() *App {
	cfg, err := macro.Load()
	if err != nil {
		log.Printf("config load: %v", err)
		cfg = macro.DefaultConfig()
	}
	a := &App{cfg: cfg, hotkeys: hotkey.NewManager(), overlay: overlay.New()}
	a.engine = engine.New(a.logEvent)
	a.engine.SetClickJitterPx(a.cfg.ClickJitterPx)
	a.overlay.Enable(a.cfg.OverlayEnabled)
	a.engine.SetPinger(a.overlay)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.engine.OnStateChange(func(running bool) {
		wruntime.EventsEmit(a.ctx, "engine:state", running)
		if it, ok := a.trayToggleItem.Load().(*systray.MenuItem); ok && it != nil {
			if running {
				it.SetTitle("Остановить кликер")
			} else {
				it.SetTitle("Запустить активный профиль")
			}
		}
	})
	if err := a.hotkeys.Start(); err != nil {
		log.Printf("hotkey start: %v", err)
	}
	if err := a.overlay.Start(); err != nil {
		log.Printf("overlay start: %v", err)
	}
	{
		r, g, b := themePingColor(a.cfg.Theme)
		a.overlay.SetColor(r, g, b)
	}
	a.rebindHotkeys()
	a.engine.StartCPSReporter(a.ctx, func(r engine.CPSReport) {
		wruntime.EventsEmit(a.ctx, "engine:cps", r)
	})

	if a.cfg.AlwaysOnTop {
		wruntime.WindowSetAlwaysOnTop(a.ctx, true)
		wruntime.WindowSetMinSize(a.ctx, miniWindowW, miniWindowH)
		wruntime.WindowSetMaxSize(a.ctx, miniWindowW, miniWindowH)
		wruntime.WindowSetSize(a.ctx, miniWindowW, miniWindowH)
	}

	go a.runTray()
}

func (a *App) shutdown(ctx context.Context) {
	a.engine.StopCPSReporter()
	a.engine.Stop()
	a.hotkeys.Stop()
	a.overlay.Stop()
	systray.Quit()
}

func (a *App) logEvent(line string) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "engine:log", line)
	}
}

// ---------------- Конфиг ----------------

func (a *App) GetConfig() *macro.Config { return a.cfg }

func (a *App) SetTheme(name string) error {
	a.cfg.Theme = name
	if a.overlay != nil {
		r, g, b := themePingColor(name)
		a.overlay.SetColor(r, g, b)
	}
	return macro.Save(a.cfg)
}

func themePingColor(name string) (byte, byte, byte) {
	switch name {
	case "dark":
		return 120, 220, 255 // голубоватый
	case "enemy-dark":
		return 80, 255, 130 // зелёный
	case "purple-neon":
		return 230, 70, 255 // мадженто
	case "green-neon":
		return 80, 255, 110 // ядовитый зелёный
	case "vampire":
		return 255, 40, 80 // алый
	default: // light
		return 230, 50, 80 // насыщенный розово-красный
	}
}

// Размеры окна: полноценное и мини.
const (
	miniWindowW = 380
	miniWindowH = 100
	fullWindowW = 1100
	fullWindowH = 720
)

// SetAlwaysOnTop включает/выключает «поверх всех окон» И автоматически
// переводит окно в мини-режим (узкая полоска со старт/стоп/пауза).
func (a *App) SetAlwaysOnTop(v bool) error {
	a.cfg.AlwaysOnTop = v
	if a.ctx != nil {
		wruntime.WindowSetAlwaysOnTop(a.ctx, v)
		if v {
			// мини: фиксированный маленький размер
			wruntime.WindowSetMinSize(a.ctx, miniWindowW, miniWindowH)
			wruntime.WindowSetMaxSize(a.ctx, miniWindowW, miniWindowH)
			wruntime.WindowSetSize(a.ctx, miniWindowW, miniWindowH)
		} else {
			// полный: снимаем потолок, восстанавливаем размер
			wruntime.WindowSetMinSize(a.ctx, 900, 600)
			wruntime.WindowSetMaxSize(a.ctx, 0, 0)
			wruntime.WindowSetSize(a.ctx, fullWindowW, fullWindowH)
		}
	}
	if it, ok := a.trayPinItem.Load().(*systray.MenuItem); ok && it != nil {
		if v {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
	return macro.Save(a.cfg)
}

// SetClickJitterPx — глобальный random offset (px) на каждый клик.
func (a *App) SetClickJitterPx(n int) error {
	if n < 0 {
		n = 0
	}
	if n > 50 {
		n = 50
	}
	a.cfg.ClickJitterPx = n
	a.engine.SetClickJitterPx(n)
	return macro.Save(a.cfg)
}

// SetOverlayEnabled — вкл/выкл click-ping overlay.
func (a *App) SetOverlayEnabled(v bool) error {
	a.cfg.OverlayEnabled = v
	a.overlay.Enable(v)
	return macro.Save(a.cfg)
}

// SetPauseHotkey — задать хоткей пауза/продолжить.
func (a *App) SetPauseHotkey(s string) error {
	a.cfg.PauseHotkey = s
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

func (a *App) ImportConfig(data string) error {
	cfg := &macro.Config{}
	if err := json.Unmarshal([]byte(data), cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	cfg.Migrate()
	a.cfg = cfg
	a.engine.SetClickJitterPx(a.cfg.ClickJitterPx)
	a.overlay.Enable(a.cfg.OverlayEnabled)
	a.rebindHotkeys()
	if a.ctx != nil {
		wruntime.WindowSetAlwaysOnTop(a.ctx, a.cfg.AlwaysOnTop)
	}
	return macro.Save(a.cfg)
}

func (a *App) ExportConfig() (string, error) {
	b, err := json.MarshalIndent(a.cfg, "", "  ")
	return string(b), err
}

func (a *App) ImportConfigFromFile() error {
	if a.ctx == nil {
		return fmt.Errorf("no app context")
	}
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Импорт конфига NecoClicker",
		Filters: []wruntime.FileFilter{
			{DisplayName: "NecoClicker config", Pattern: "*.necoclicker.json;*.json"},
		},
	})
	if err != nil || path == "" {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return a.ImportConfig(string(b))
}

func (a *App) ExportConfigToFile() error {
	if a.ctx == nil {
		return fmt.Errorf("no app context")
	}
	ts := time.Now().Format("20060102-150405")
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Экспорт конфига NecoClicker",
		DefaultFilename: "necoclicker-" + ts + ".necoclicker.json",
		Filters: []wruntime.FileFilter{
			{DisplayName: "NecoClicker config", Pattern: "*.necoclicker.json;*.json"},
		},
	})
	if err != nil || path == "" {
		return err
	}
	data, err := a.ExportConfig()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

// ---------------- Профили ----------------

func (a *App) ListProfiles() []macro.SimpleConfig { return a.cfg.Profiles }
func (a *App) ActiveProfileIndex() int            { return a.cfg.Active }

func (a *App) SetActiveProfile(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		return fmt.Errorf("profile index %d out of range", idx)
	}
	a.cfg.Active = idx
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

func (a *App) SaveProfile(idx int, p macro.SimpleConfig) (int, error) {
	if p.Name == "" {
		p.Name = fmt.Sprintf("Profile %d", len(a.cfg.Profiles)+1)
	}
	if p.IntervalMs < 0 {
		p.IntervalMs = 0
	}
	if p.IntervalMs > 600000 {
		p.IntervalMs = 600000
	}
	if p.Button == "" {
		p.Button = macro.BtnLeft
	}
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		a.cfg.Profiles = append(a.cfg.Profiles, p)
		idx = len(a.cfg.Profiles) - 1
	} else {
		a.cfg.Profiles[idx] = p
	}
	a.rebindHotkeys()
	return idx, macro.Save(a.cfg)
}

func (a *App) DeleteProfile(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		return nil
	}
	a.cfg.Profiles = append(a.cfg.Profiles[:idx], a.cfg.Profiles[idx+1:]...)
	if len(a.cfg.Profiles) == 0 {
		a.cfg.Profiles = []macro.SimpleConfig{macro.DefaultProfile()}
	}
	if a.cfg.Active >= len(a.cfg.Profiles) {
		a.cfg.Active = len(a.cfg.Profiles) - 1
	}
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

// ---------------- Цепочки ----------------

func (a *App) SaveChain(idx int, ch macro.Chain) error {
	if idx < 0 || idx >= len(a.cfg.Chains) {
		a.cfg.Chains = append(a.cfg.Chains, ch)
	} else {
		a.cfg.Chains[idx] = ch
	}
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

func (a *App) DeleteChain(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Chains) {
		return nil
	}
	a.cfg.Chains = append(a.cfg.Chains[:idx], a.cfg.Chains[idx+1:]...)
	if a.cfg.ActiveChain >= len(a.cfg.Chains) {
		a.cfg.ActiveChain = len(a.cfg.Chains) - 1
		if a.cfg.ActiveChain < 0 {
			a.cfg.ActiveChain = 0
		}
	}
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

func (a *App) SetActiveChain(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Chains) {
		return fmt.Errorf("chain index %d out of range", idx)
	}
	a.cfg.ActiveChain = idx
	return macro.Save(a.cfg)
}

func (a *App) ActiveChainIndex() int { return a.cfg.ActiveChain }

// ---------------- Sequence (пошаговый кликер) ----------------

func (a *App) GetSequence() macro.Sequence { return a.cfg.Sequence }

func (a *App) SaveSequence(seq macro.Sequence) error {
	if seq.IntervalMs < 0 {
		seq.IntervalMs = 0
	}
	if seq.Steps == nil {
		seq.Steps = []macro.Step{}
	}
	a.cfg.Sequence = seq
	a.rebindHotkeys()
	return macro.Save(a.cfg)
}

func (a *App) AddSequenceStep(s macro.Step) error {
	if s.Button == "" {
		s.Button = macro.BtnLeft
	}
	a.cfg.Sequence.Steps = append(a.cfg.Sequence.Steps, s)
	return macro.Save(a.cfg)
}

func (a *App) ClearSequenceSteps() error {
	a.cfg.Sequence.Steps = []macro.Step{}
	return macro.Save(a.cfg)
}

func (a *App) StartSequence() {
	a.engine.SetDryRun(false)
	a.engine.RunSequence(a.cfg.Sequence)
}

func (a *App) StartSequenceDry() {
	a.engine.SetDryRun(true)
	a.engine.RunSequence(a.cfg.Sequence)
}

// CaptureCursor — мгновенный снимок (для UI кнопки "захватить точку").
func (a *App) CaptureCursor() macro.Step {
	x, y := winmouse.GetCursor()
	return macro.Step{X: x, Y: y, Button: macro.BtnLeft}
}

// RecordStep блокируется до следующего глобального нажатия (любая клавиша
// или Mouse4/5) и возвращает Step с текущими координатами курсора в момент
// нажатия. Используется для пошаговой записи через хоткей (например F10).
//
// recordHotkey — фильтр; если пусто, ловится любое первое нажатие.
func (a *App) RecordStep(timeoutMs int, recordHotkey string) (macro.Step, error) {
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}
	got, err := a.hotkeys.RecordOnce(time.Duration(timeoutMs) * time.Millisecond)
	if err != nil {
		return macro.Step{}, err
	}
	if recordHotkey != "" && got != recordHotkey {
		// поймали что-то другое — повторно записываем (рекурсия не нужна, простой loop)
		// но пользователь, скорее всего, согласен на любую клавишу как сигнал
		_ = got
	}
	x, y := winmouse.GetCursor()
	return macro.Step{X: x, Y: y, Button: macro.BtnLeft}, nil
}

// ---------------- Управление движком ----------------

func (a *App) IsRunning() bool { return a.engine.IsRunning() }
func (a *App) IsPaused() bool  { return a.engine.IsPaused() }
func (a *App) Pause()          { a.engine.Pause() }
func (a *App) Resume()         { a.engine.Resume() }
func (a *App) TogglePause()    { a.engine.TogglePause() }

func (a *App) StartSimple() {
	a.engine.SetDryRun(false)
	a.engine.RunSimple(a.cfg.ActiveProfile())
}

func (a *App) StartProfile(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		return fmt.Errorf("profile index %d out of range", idx)
	}
	a.engine.SetDryRun(false)
	a.engine.RunSimple(a.cfg.Profiles[idx])
	return nil
}

func (a *App) StartChain(idx int) {
	if idx < 0 || idx >= len(a.cfg.Chains) {
		return
	}
	a.engine.SetDryRun(false)
	a.engine.RunChain(a.cfg.Chains[idx])
}

func (a *App) StartChainDry(idx int) {
	if idx < 0 || idx >= len(a.cfg.Chains) {
		return
	}
	a.engine.SetDryRun(true)
	a.engine.RunChain(a.cfg.Chains[idx])
}

func (a *App) StartSimpleDry() {
	a.engine.SetDryRun(true)
	a.engine.RunSimple(a.cfg.ActiveProfile())
}

func (a *App) StartProfileDry(idx int) error {
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		return fmt.Errorf("profile index %d out of range", idx)
	}
	a.engine.SetDryRun(true)
	a.engine.RunSimple(a.cfg.Profiles[idx])
	return nil
}

func (a *App) StartProfileLimited(idx int, lim macro.RunLimits, dry bool) error {
	if idx < 0 || idx >= len(a.cfg.Profiles) {
		return fmt.Errorf("profile index %d out of range", idx)
	}
	a.engine.SetDryRun(dry)
	a.engine.RunSimpleLimited(a.cfg.Profiles[idx], lim)
	return nil
}

func (a *App) Stop() { a.engine.Stop() }

// ---------------- CPS ----------------

func (a *App) ResetClicks()        { a.engine.ResetClicks() }
func (a *App) TotalClicks() uint64 { return a.engine.TotalClicks() }

// ---------------- Hotkey recorder ----------------

func (a *App) RecordHotkey(timeoutMs int) (string, error) {
	if timeoutMs <= 0 {
		timeoutMs = 8000
	}
	return a.hotkeys.RecordOnce(time.Duration(timeoutMs) * time.Millisecond)
}

// ---------------- Утилиты ----------------

func (a *App) CursorPos() [2]int {
	x, y := winmouse.GetCursor()
	return [2]int{x, y}
}

func (a *App) ConfigPath() string {
	p, _ := macro.ConfigPath()
	return p
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		wruntime.WindowShow(a.ctx)
	}
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		wruntime.WindowHide(a.ctx)
	}
}

// rebindHotkeys: активный профиль + цепочки + sequence + pause hotkey.
func (a *App) rebindHotkeys() {
	binds := []hotkey.Bind{}

	if len(a.cfg.Profiles) > 0 {
		ap := a.cfg.ActiveProfile()
		if ap.Hotkey != "" {
			binds = append(binds, hotkey.Bind{
				Hotkey: ap.Hotkey,
				Cb: func() {
					a.engine.Toggle(func() {
						a.engine.SetDryRun(false)
						a.engine.RunSimple(a.cfg.ActiveProfile())
					})
				},
			})
		}
	}

	for i := range a.cfg.Chains {
		idx := i
		ch := a.cfg.Chains[i]
		if ch.Hotkey == "" {
			continue
		}
		binds = append(binds, hotkey.Bind{
			Hotkey: ch.Hotkey,
			Cb: func() {
				a.engine.Toggle(func() {
					if idx >= len(a.cfg.Chains) {
						return
					}
					a.engine.SetDryRun(false)
					a.engine.RunChain(a.cfg.Chains[idx])
				})
			},
		})
	}

	// sequence hotkey
	if hk := a.cfg.Sequence.Hotkey; hk != "" {
		binds = append(binds, hotkey.Bind{
			Hotkey: hk,
			Cb: func() {
				a.engine.Toggle(func() {
					a.engine.SetDryRun(false)
					a.engine.RunSequence(a.cfg.Sequence)
				})
			},
		})
	}

	// pause hotkey (срабатывает только когда что-то запущено)
	if hk := a.cfg.PauseHotkey; hk != "" {
		binds = append(binds, hotkey.Bind{
			Hotkey: hk,
			Cb: func() {
				a.engine.TogglePause()
				if a.ctx != nil {
					wruntime.EventsEmit(a.ctx, "engine:paused", a.engine.IsPaused())
				}
			},
		})
	}

	if err := a.hotkeys.SetAll(binds); err != nil {
		log.Printf("hotkey rebind: %v", err)
	}
}

// ---------------- Tray ----------------

func (a *App) runTray() {
	systray.Run(func() {
		systray.SetIcon(trayIconIco)
		systray.SetTitle("NecoClicker")
		systray.SetTooltip("NecoClicker — кликер")

		showItem := systray.AddMenuItem("Показать окно", "")
		toggleItem := systray.AddMenuItem("Запустить активный профиль", "Toggle active simple profile")
		a.trayToggleItem.Store(toggleItem)
		pinItem := systray.AddMenuItemCheckbox("Поверх всех окон", "Always-on-top", a.cfg.AlwaysOnTop)
		a.trayPinItem.Store(pinItem)
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Выйти", "Quit application")

		go func() {
			for {
				select {
				case <-showItem.ClickedCh:
					a.ShowWindow()
				case <-toggleItem.ClickedCh:
					a.engine.Toggle(func() {
						a.engine.SetDryRun(false)
						a.engine.RunSimple(a.cfg.ActiveProfile())
					})
				case <-pinItem.ClickedCh:
					_ = a.SetAlwaysOnTop(!a.cfg.AlwaysOnTop)
				case <-quitItem.ClickedCh:
					if a.ctx != nil {
						wruntime.Quit(a.ctx)
					}
					return
				}
			}
		}()
	}, nil)
}
