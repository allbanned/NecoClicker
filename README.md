# NecoClicker

Лёгкий автокликер для Windows с современным UI и глобальными хоткеями.

## Стек

- **Go 1.22+** — backend.
- **Wails v2** — нативный десктоп через WebView2 (на Win11 предустановлен).
- **React 18 + TypeScript + Vite 6** — фронт.
- **Tailwind v3 + shadcn-style компоненты** + Radix UI primitives.
- **WinAPI напрямую** через `syscall` (без CGO-зависимостей типа robotgo):
  - `user32!SendInput` — точные клики.
  - `WH_KEYBOARD_LL` (низкоуровневый клавиатурный хук) — глобальные хоткеи.
- **Per-Monitor V2 DPI awareness** — координаты не плывут на HiDPI-мониторах.

## Артефакты

```
build/bin/NecoClicker.exe                  ~3.4 MB   portable single-file
build/bin/NecoClicker-amd64-installer.exe  ~6.5 MB   NSIS installer
```

`NecoClicker.exe` можно отдавать кому угодно — никаких DLL, никаких зависимостей. WebView2 runtime есть на всех Win10 (v1803+) и Win11 из коробки.

## Сборка с нуля

### Тулчейн

```bat
:: Go 1.22+
winget install -e --id GoLang.Go

:: Node 20+ и pnpm
winget install -e --id OpenJS.NodeJS.LTS
npm install -g pnpm

:: MinGW-w64 (CGO для WebView2 и low-level WinAPI)
winget install -e --id BrechtSanders.WinLibs.POSIX.UCRT

:: Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

:: Опционально — для меньшего exe и установщика
winget install -e --id UPX.UPX
winget install -e --id NSIS.NSIS

:: Проверка
wails doctor
```

### Билд

```bat
build.bat
```

или вручную:

```bat
wails build -clean -nsis -upx -platform windows/amd64 -ldflags "-s -w"
```

### Hot-reload разработка

```bat
wails dev
```

## Структура проекта

```
NecoClicker/
├── main.go                       — Wails entry, опции окна
├── app.go                        — App struct: bindings, методы, события
├── wails.json                    — конфиг Wails (имя exe, frontend cmd)
├── go.mod
├── build.bat                     — full-release pipeline
├── build/
│   ├── appicon.png               — 1024×1024, embed в exe
│   ├── windows/icon.ico          — multi-size ICO для exe иконки
│   └── bin/                      — выходные .exe
├── internal/                     — pure-Go backend (без UI)
│   ├── dpi/                      — Per-Monitor V2 awareness
│   ├── winmouse/                 — user32!SendInput / SetCursorPos
│   ├── hotkey/                   — WH_KEYBOARD_LL hook + парсер строк
│   ├── macro/                    — модель Action/Chain + JSON storage
│   └── engine/                   — выполнение в goroutine с context-cancel
└── frontend/
    ├── index.html
    ├── package.json
    ├── tailwind.config.js
    ├── vite.config.ts
    ├── wailsjs/                  — авто-генерируемые JS/TS bindings
    └── src/
        ├── App.tsx               — корневой shell (sidebar + main)
        ├── main.tsx              — entry
        ├── index.css             — Tailwind + 6 тем (HSL переменные)
        ├── lib/utils.ts          — cn() helper
        ├── hooks/                — useConfig, useEngine
        ├── components/
        │   ├── theme-provider.tsx
        │   ├── theme-picker.tsx
        │   ├── app-sidebar.tsx
        │   └── ui/               — shadcn-style: button, input, card, ...
        ├── assets/neco.png       — маскот
        └── pages/
            ├── home.tsx          — Главная (simple clicker)
            ├── macros.tsx        — Редактор цепочек
            ├── sandbox.tsx       — Тест (dry-run + лог)
            ├── themes.tsx        — Выбор темы + превью
            └── settings.tsx      — Хоткеи / путь конфига / about
```

## Темы

6 тем, переключаются мгновенно через `data-theme` атрибут на `<html>`. При первом запуске берётся системная (`prefers-color-scheme`).

| ID            | Описание                                       |
|---------------|------------------------------------------------|
| `light`       | Светлый нейтральный                            |
| `dark`        | Классический тёмный                            |
| `enemy-dark`  | Почти-OLED-чёрный, минимум контраста           |
| `purple-neon` | Ультра-фиолетовый неон                         |
| `green-neon`  | Кислотно-зелёный матрица                       |
| `vampire`     | Алый/тёмно-винный, ночной                      |

Все цвета — HSL CSS-переменные в `frontend/src/index.css`. Хочешь свою — добавляешь блок и регистрируешь в `theme-provider.tsx`.

## Как это работает (ключевые места)

### Глобальные хоткеи без `RegisterHotKey`

`internal/hotkey/hotkey_windows.go` — отдельный OS-thread (`runtime.LockOSThread`) с собственным message-loop (`GetMessageW`), на нём висит низкоуровневый хук `WH_KEYBOARD_LL`. Хук **не глотает** нажатия — фокусированное приложение всё ещё их получает. Состояние модификаторов читается синхронно через `GetAsyncKeyState`. Остановка — `PostThreadMessageW(WM_QUIT)`.

### Точное управление мышью

`internal/winmouse/mouse_windows.go` — `SendInput` с явно объявленной структурой `INPUT` (40 байт на x64 с 4-байтовым padding после `Type`). Это правильнее устаревшего `mouse_event` и работает в Per-Monitor V2 DPI без искажения координат.

### Безопасное завершение

`engine.Engine` каждое исполнение оборачивает в `context.WithCancel`. `Stop()` дёргает cancel + `wg.Wait()` дожидается выхода goroutine. `Toggle(start)` атомарно: запущено → стоп, иначе → старт.

### React ↔ Go события

Бэкенд эмитит `engine:state` (running ↔ idle) и `engine:log` (текстовые строки). Хук `useEngine()` слушает их через `wailsjs/runtime` — sidebar мигает индикатором, sandbox-страница стримит лог.

## Конфиг

`%APPDATA%\NecoClicker\config.json` — единый файл. Пример:

```json
{
  "profiles": [
    {
      "name": "Default",
      "button": "left",
      "interval_ms": 100,
      "use_current": true,
      "x": 0, "y": 0,
      "hotkey": "F6"
    },
    {
      "name": "Max speed",
      "button": "left",
      "interval_ms": 0,
      "use_current": true,
      "hotkey": "F7"
    }
  ],
  "active": 0,
  "chains": [
    {
      "name": "AFK farm",
      "hotkey": "Ctrl+F1",
      "loops": 0,
      "actions": [
        {"type": "click",  "button": "left", "use_current": true},
        {"type": "delay",  "delay_ms": 250},
        {"type": "move",   "x": 200, "y": 0, "relative": true},
        {"type": "click",  "button": "x1", "x": 1280, "y": 720}
      ]
    }
  ],
  "theme": "enemy-dark"
}
```

`interval_ms` — `float`. Поддерживаются доли миллисекунды (`0.5`, `0.001`). `0` означает максимальную скорость без сна (tight loop, 100% одного ядра CPU). Глобальный хоткей привязан только к **активному** профилю (`active` — индекс в `profiles`).

Поддерживаемые кнопки: `left`, `right`, `middle`, `x1` (Mouse4), `x2` (Mouse5).

## Хоткеи

Парсер `internal/hotkey.Parse` принимает строки вида:

- Модификаторы: `Ctrl`, `Alt`, `Shift`, `Win` (любая комбинация через `+`)
- Клавиши: `A`–`Z`, `0`–`9`, `F1`–`F24`, `Space`, `Enter`, `Tab`, `Esc`, `Insert`, `Delete`, `Home`, `End`, `PgUp`, `PgDn`, `Up`/`Down`/`Left`/`Right`

Примеры: `F6`, `Ctrl+Shift+F1`, `Alt+Q`, `Win+Insert`.

## Известные ограничения

- Хоткеи не работают на защищённых системой комбинациях (`Win+L`, `Ctrl+Alt+Del`) — это by design Windows.
- Если кликаешь в окно, запущенное от админа — кликер тоже должен быть от админа (UAC isolation).
- Минимальный интервал 1 мс — ниже физически не имеет смысла.
