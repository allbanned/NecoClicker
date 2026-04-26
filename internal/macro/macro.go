package macro

type ActionType string

const (
	ActionClick ActionType = "click"
	ActionMove  ActionType = "move"
	ActionDelay ActionType = "delay"
	ActionDrag  ActionType = "drag"
)

type MouseButton string

const (
	BtnLeft   MouseButton = "left"
	BtnRight  MouseButton = "right"
	BtnMiddle MouseButton = "middle"
	BtnX1     MouseButton = "x1" // боковая "вперёд" (Mouse4)
	BtnX2     MouseButton = "x2" // боковая "назад"  (Mouse5)
)

// Action — единичный шаг макроса.
//
// Для type=drag: (X,Y) → (EndX,EndY) с зажатой Button за DurationMs.
// Если DurationMs=0 — moment-drag (мгновенно перетянуть).
type Action struct {
	Type       ActionType  `json:"type"`
	X          int         `json:"x,omitempty"`
	Y          int         `json:"y,omitempty"`
	EndX       int         `json:"end_x,omitempty"`
	EndY       int         `json:"end_y,omitempty"`
	Relative   bool        `json:"relative,omitempty"`
	UseCurrent bool        `json:"use_current,omitempty"`
	Button     MouseButton `json:"button,omitempty"`
	DelayMs    int         `json:"delay_ms,omitempty"`
	DurationMs int         `json:"duration_ms,omitempty"`
}

// Step — простой шаг для пошагового кликера: точка + кнопка.
type Step struct {
	X      int         `json:"x"`
	Y      int         `json:"y"`
	Button MouseButton `json:"button"`
}

// Sequence — последовательность кликов "по точкам".
type Sequence struct {
	Steps      []Step  `json:"steps"`
	IntervalMs float64 `json:"interval_ms"` // задержка между шагами
	Loops      int     `json:"loops"`       // 0 = бесконечно
	Hotkey     string  `json:"hotkey"`
}

func DefaultSequence() Sequence {
	return Sequence{
		Steps:      []Step{},
		IntervalMs: 200,
		Loops:      0,
		Hotkey:     "",
	}
}

type Chain struct {
	Name    string   `json:"name"`
	Hotkey  string   `json:"hotkey,omitempty"`
	Loops   int      `json:"loops"`
	Actions []Action `json:"actions"`
}

// SimpleConfig — настройки одного "профиля" простого кликера.
// Один Config может хранить несколько таких профилей (см. Config.Profiles).
//
// IntervalMs — float, поддерживает доли миллисекунды (например, 0.5).
// Значение 0 = "максимальная скорость" (tight-loop без сна).
type SimpleConfig struct {
	Name       string      `json:"name"`
	Button     MouseButton `json:"button"`
	IntervalMs float64     `json:"interval_ms"`
	UseCurrent bool        `json:"use_current"`
	X          int         `json:"x"`
	Y          int         `json:"y"`
	Hotkey     string      `json:"hotkey"`
}

// RunLimits — общие ограничения на запуск (для "Таймера" и "Jitter").
//   - DurationSec=0 + MaxClicks=0 → без лимита
//   - DurationSec>0 → остановиться через N секунд
//   - MaxClicks>0   → остановиться после N кликов
//   - Если оба заданы — кто первый сработает, тот и останавливает
//
// JitterMs — рандомизация интервала. На каждый клик берётся случайное
// смещение из [-JitterMs/2, +JitterMs/2] и прибавляется к базовому интервалу.
type RunLimits struct {
	DurationSec int     `json:"duration_sec,omitempty"`
	MaxClicks   uint64  `json:"max_clicks,omitempty"`
	JitterMs    float64 `json:"jitter_ms,omitempty"`
}

type Config struct {
	Profiles      []SimpleConfig `json:"profiles"`
	Active        int            `json:"active"`
	Chains        []Chain        `json:"chains"`
	ActiveChain   int            `json:"active_chain"`
	Sequence      Sequence       `json:"sequence"`
	Theme         string         `json:"theme"`
	AlwaysOnTop   bool           `json:"always_on_top"`

	// Глобальное случайное смещение каждого клика на ±N пикселей.
	// Для всех источников (Simple, Chain, Sequence). 0 = выкл.
	ClickJitterPx int `json:"click_jitter_px"`

	// Click-ping overlay: рисовать прозрачную вспышку в точке клика.
	OverlayEnabled bool `json:"overlay_enabled"`

	// Хоткей для пауза/возобновить (отдельный от пуск/стоп).
	PauseHotkey string `json:"pause_hotkey"`

	// Legacy: одиночный профиль из v1.0. На загрузке мигрируется в Profiles[0].
	LegacySimple *SimpleConfig `json:"simple,omitempty"`
}

// ActiveProfile возвращает текущий активный профиль (или дефолтный если ничего нет).
func (c *Config) ActiveProfile() SimpleConfig {
	if len(c.Profiles) == 0 {
		return DefaultProfile()
	}
	idx := c.Active
	if idx < 0 || idx >= len(c.Profiles) {
		idx = 0
	}
	return c.Profiles[idx]
}

func DefaultProfile() SimpleConfig {
	return SimpleConfig{
		Name:       "Default",
		Button:     BtnLeft,
		IntervalMs: 100,
		UseCurrent: true,
		Hotkey:     "F6",
	}
}

func DefaultConfig() *Config {
	return &Config{
		Profiles:       []SimpleConfig{DefaultProfile()},
		Active:         0,
		Chains:         []Chain{},
		Sequence:       DefaultSequence(),
		Theme:          "", // пусто = фронт сам подберёт по системной (auto)
		OverlayEnabled: true,
		PauseHotkey:    "F8",
	}
}

// Migrate выполняет апгрейд старого формата (v1.0) к новому.
func (c *Config) Migrate() {
	if c.LegacySimple != nil {
		p := *c.LegacySimple
		if p.Name == "" {
			p.Name = "Default"
		}
		if len(c.Profiles) == 0 {
			c.Profiles = []SimpleConfig{p}
			c.Active = 0
		}
		c.LegacySimple = nil
	}
	if len(c.Profiles) == 0 {
		c.Profiles = []SimpleConfig{DefaultProfile()}
		c.Active = 0
	}
	if c.Active < 0 || c.Active >= len(c.Profiles) {
		c.Active = 0
	}
	if c.Chains == nil {
		c.Chains = []Chain{}
	}
	if c.Sequence.Steps == nil {
		c.Sequence = DefaultSequence()
	}
	if c.PauseHotkey == "" {
		c.PauseHotkey = "F8"
	}
	// гарантируем имя у каждого профиля
	for i := range c.Profiles {
		if c.Profiles[i].Name == "" {
			c.Profiles[i].Name = "Profile " + itoa(i+1)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
