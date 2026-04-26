import { useEffect, useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  ConfigPath, ImportConfigFromFile, ExportConfigToFile,
  SetClickJitterPx, SetOverlayEnabled, SetPauseHotkey,
} from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import {
  Folder, Keyboard, Info, Github, User, ExternalLink, Download, Upload, Database,
  Sparkles, Pause, Eye,
} from 'lucide-react'
import { useConfig } from '@/components/config-provider'
import { useConfirm } from '@/components/confirm-dialog'
import { HotkeyRecorder } from '@/components/hotkey-recorder'
import necoUrl from '@/assets/neco.png'

const GITHUB_URL = 'https://github.com/allbanned/NecoClicker'
const AUTHOR = 'allbanned'

export function SettingsPage() {
  const [path, setPath] = useState('')
  useEffect(() => { ConfigPath().then(setPath) }, [])
  const { cfg, reload } = useConfig()
  const { ask, alert: showAlert } = useConfirm()

  const jitter = cfg?.click_jitter_px ?? 0
  const overlayOn = cfg?.overlay_enabled ?? true
  const pauseHk = cfg?.pause_hotkey ?? 'F8'

  const onImport = async () => {
    const ok = await ask({
      title: 'Импортировать конфиг?',
      description: 'Текущие профили, цепочки и настройки будут заменены данными из выбранного файла.',
      confirmText: 'Импортировать',
      destructive: true,
    })
    if (!ok) return
    try {
      await ImportConfigFromFile()
      await reload()
      await showAlert('Готово', 'Конфиг успешно импортирован.')
    } catch (e: any) {
      await showAlert('Ошибка импорта', String(e?.message || e))
    }
  }

  const onExport = async () => {
    try {
      await ExportConfigToFile()
    } catch (e: any) {
      await showAlert('Ошибка экспорта', String(e?.message || e))
    }
  }

  return (
    <div className="space-y-6">
      {/* Поведение кликера */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-primary" /> Поведение
          </CardTitle>
          <CardDescription>Глобальные настройки кликов и UI.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Click jitter */}
          <div>
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="cj">Случайное смещение клика, ±N px</Label>
              <span className="font-mono text-xs text-primary">{jitter}</span>
            </div>
            <input
              id="cj"
              type="range"
              min="0"
              max="20"
              step="1"
              value={jitter}
              onChange={async (e) => { await SetClickJitterPx(parseInt(e.target.value)); reload() }}
              className="mt-2 w-full accent-primary"
            />
            <p className="mt-1 text-[11px] text-muted-foreground">
              Каждый клик уходит в случайную точку в радиусе ±N пикселей от заданной. Помогает обходить детекторы "идеально совпавших координат". 0 = выключено.
            </p>
          </div>

          {/* Overlay */}
          <label className={`flex items-center justify-between gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors ${overlayOn ? 'border-primary/40 bg-primary/5' : 'bg-muted/30'}`}>
            <div className="flex items-start gap-2">
              <Eye className={`mt-0.5 h-4 w-4 shrink-0 ${overlayOn ? 'text-primary' : 'text-muted-foreground'}`} />
              <div>
                <div className="font-medium text-foreground">Click-ping overlay</div>
                <div className="text-xs text-muted-foreground">Прозрачная вспышка прямо на экране в точке клика. Видно что бот делает.</div>
              </div>
            </div>
            <Switch checked={overlayOn} onCheckedChange={async (v) => { await SetOverlayEnabled(v); reload() }} />
          </label>

          {/* Pause hotkey */}
          <div>
            <Label className="mb-1.5 flex items-center gap-1.5">
              <Pause className="h-3.5 w-3.5" /> Хоткей паузы / возобновления
            </Label>
            <HotkeyRecorder
              value={pauseHk}
              onChange={async (v) => { await SetPauseHotkey(v); reload() }}
              placeholder="F8"
            />
            <p className="mt-1 text-[11px] text-muted-foreground">
              Замораживает выполнение в любой момент — кликер ждёт пока не нажмёшь снова. Отдельно от пуск/стоп активного профиля.
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-5 w-5 text-primary" /> Импорт / экспорт
          </CardTitle>
          <CardDescription>
            Перенос профилей и цепочек на другой ПК или бэкап. Формат — `.necoclicker.json`.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={onExport}>
            <Download className="h-4 w-4" /> Экспортировать конфиг
          </Button>
          <Button variant="outline" onClick={onImport}>
            <Upload className="h-4 w-4" /> Импортировать конфиг
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Folder className="h-5 w-5 text-primary" /> Расположение конфига
          </CardTitle>
          <CardDescription>Все настройки и цепочки хранятся в одном JSON-файле.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border bg-muted/30 p-3 font-mono text-xs break-all">{path || '...'}</div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Keyboard className="h-5 w-5 text-primary" /> Глобальные хоткеи
          </CardTitle>
          <CardDescription>Работают даже когда окно свёрнуто.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="grid gap-2 text-xs sm:grid-cols-2">
            <Section title="Модификаторы">Ctrl, Alt, Shift, Win</Section>
            <Section title="Буквы / цифры">A–Z, 0–9</Section>
            <Section title="Функциональные">F1–F24</Section>
            <Section title="Спец.">Space, Enter, Tab, Esc</Section>
            <Section title="Навигация">Insert, Delete, Home, End, PgUp, PgDn</Section>
            <Section title="Стрелки">Up, Down, Left, Right</Section>
          </div>
          <div className="mt-3 rounded-md border border-dashed bg-muted/20 p-3 text-xs text-muted-foreground">
            <b className="text-foreground">Пример:</b> <code className="rounded bg-background px-1 py-0.5 font-mono">Ctrl+Shift+F1</code>,&nbsp;
            <code className="rounded bg-background px-1 py-0.5 font-mono">Alt+Q</code>,&nbsp;
            <code className="rounded bg-background px-1 py-0.5 font-mono">F6</code>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Info className="h-5 w-5 text-primary" /> О приложении
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-4">
            <img src={necoUrl} alt="" className="h-20 w-20 rounded-xl border border-border object-cover" />
            <div className="space-y-1 text-sm">
              <div className="text-base font-semibold">NecoClicker <span className="text-muted-foreground">v1.6.4</span></div>
              <div className="text-xs text-muted-foreground">
                Лёгкий автокликер с глобальными хоткеями и редактором макросов.<br />
                Go · Wails · React · Tailwind.
              </div>
            </div>
          </div>

          <div className="grid gap-2 sm:grid-cols-2">
            <div className="rounded-md border bg-card/50 p-3">
              <div className="mb-1 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                <User className="h-3 w-3" /> Автор
              </div>
              <div className="font-mono text-sm">{AUTHOR}</div>
              <div className="mt-1 text-[10px] text-muted-foreground italic">соцсети — позже</div>
            </div>

            <button
              onClick={() => BrowserOpenURL(GITHUB_URL)}
              className="group rounded-md border bg-card/50 p-3 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
            >
              <div className="mb-1 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                <Github className="h-3 w-3" /> GitHub
                <ExternalLink className="ml-auto h-3 w-3 opacity-0 transition-opacity group-hover:opacity-100" />
              </div>
              <div className="break-all font-mono text-xs group-hover:text-primary">{GITHUB_URL.replace('https://', '')}</div>
            </button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-md border bg-card/50 p-2.5">
      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{title}</div>
      <div className="font-mono text-foreground">{children}</div>
    </div>
  )
}
