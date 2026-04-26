import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Footprints, Crosshair, Play, Square, Trash2, Loader2, Plus, Repeat, Keyboard,
} from 'lucide-react'
import {
  GetSequence, SaveSequence, AddSequenceStep, ClearSequenceSteps,
  StartSequence, StartSequenceDry, Stop, CaptureCursor,
} from '../../wailsjs/go/main/App'
import { macro } from '../../wailsjs/go/models'
import { useEngine } from '@/components/engine-provider'
import { useConfirm } from '@/components/confirm-dialog'
import { HotkeyRecorder } from '@/components/hotkey-recorder'
import { cn } from '@/lib/utils'

const MAX_STEPS = 10
const RECORD_HOTKEY = 'F10' // hardcoded для простоты UX

const BUTTON_OPTIONS = [
  { id: 'left', label: 'ЛКМ' },
  { id: 'right', label: 'ПКМ' },
  { id: 'middle', label: 'СКМ' },
  { id: 'x1', label: 'X1' },
  { id: 'x2', label: 'X2' },
]

export function StepsPage() {
  const { running } = useEngine()
  const { ask } = useConfirm()
  const [seq, setSeq] = useState<macro.Sequence | null>(null)
  const [recording, setRecording] = useState(false)
  const [recordError, setRecordError] = useState<string | null>(null)

  const reload = async () => setSeq(await GetSequence())
  useEffect(() => { reload() }, [])

  if (!seq) return null
  const steps = seq.steps ?? []

  const updateStep = async (i: number, patch: Partial<macro.Step>) => {
    const next = [...steps]
    next[i] = new macro.Step({ ...steps[i], ...patch } as any)
    const updated = new macro.Sequence({ ...seq, steps: next } as any)
    await SaveSequence(updated)
    setSeq(updated)
  }

  const removeStep = async (i: number) => {
    const next = steps.filter((_, k) => k !== i)
    const updated = new macro.Sequence({ ...seq, steps: next } as any)
    await SaveSequence(updated)
    setSeq(updated)
  }

  const addCurrent = async () => {
    if (steps.length >= MAX_STEPS) return
    const s = await CaptureCursor()
    await AddSequenceStep(s)
    await reload()
  }

  const startRecording = async () => {
    if (recording) return
    setRecording(true)
    setRecordError(null)
    try {
      // цикл записи: после каждого нажатия добавляется шаг и снова ждём
      while (true) {
        const cur = await GetSequence()
        if ((cur.steps?.length ?? 0) >= MAX_STEPS) break
        try {
          // RecordHotkey возвращает строку при любом первом нажатии — нам её содержимое неважно,
          // важен сам факт нажатия + текущая позиция курсора
          const got = await (await import('../../wailsjs/go/main/App')).RecordHotkey(60000)
          // Esc → выходим
          if (!got || got === 'Esc' || got === 'Escape') break
          const s = await CaptureCursor()
          await AddSequenceStep(s)
          await reload()
        } catch (e: any) {
          // таймаут или другая ошибка — выходим из цикла
          break
        }
      }
    } finally {
      setRecording(false)
    }
  }

  const clearAll = async () => {
    if (steps.length === 0) return
    const ok = await ask({
      title: 'Очистить все шаги?',
      description: `${steps.length} шагов будет удалено.`,
      confirmText: 'Очистить',
      destructive: true,
    })
    if (!ok) return
    await ClearSequenceSteps()
    await reload()
  }

  const setIntervalMs = async (v: string) => {
    const ms = Math.max(0, parseFloat(v.replace(',', '.')) || 0)
    const updated = new macro.Sequence({ ...seq, interval_ms: ms } as any)
    await SaveSequence(updated)
    setSeq(updated)
  }
  const setLoops = async (v: string) => {
    const n = Math.max(0, parseInt(v) || 0)
    const updated = new macro.Sequence({ ...seq, loops: n } as any)
    await SaveSequence(updated)
    setSeq(updated)
  }
  const setHotkey = async (v: string) => {
    const updated = new macro.Sequence({ ...seq, hotkey: v } as any)
    await SaveSequence(updated)
    setSeq(updated)
  }

  const start = async () => {
    if (steps.length === 0) return
    await StartSequence()
  }

  return (
    <div className="grid h-[calc(100vh-9rem)] gap-4 lg:grid-cols-[1fr_360px]">
      <Card className="flex min-h-0 flex-col overflow-hidden">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Footprints className="h-5 w-5 text-primary" /> По точкам
          </CardTitle>
          <CardDescription>
            Запиши до {MAX_STEPS} точек: наведи курсор и нажимай <span className="font-mono text-foreground">{RECORD_HOTKEY}</span>.
            После записи можно поменять кнопку для каждого шага.
          </CardDescription>
        </CardHeader>

        {/* RECORDING BANNER */}
        <div className="px-5 pb-3">
          {recording ? (
            <div className="flex items-center gap-3 rounded-lg border border-primary/60 bg-primary/10 p-4 glow-primary">
              <Loader2 className="h-5 w-5 animate-spin text-primary" />
              <div className="flex-1">
                <div className="font-semibold text-primary">Запись активна</div>
                <div className="text-xs text-muted-foreground">
                  Наведи курсор и жми <span className="font-mono text-foreground">{RECORD_HOTKEY}</span>.
                  Esc — закончить. Шагов осталось: <b className="text-foreground">{MAX_STEPS - steps.length}</b>
                </div>
              </div>
              <Button variant="outline" onClick={() => setRecording(false)}>Стоп</Button>
            </div>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="neon"
                onClick={startRecording}
                disabled={steps.length >= MAX_STEPS}
              >
                <Crosshair className="h-4 w-4" />
                {steps.length === 0 ? 'Начать запись' : 'Продолжить запись'}
              </Button>
              <Button variant="outline" onClick={addCurrent} disabled={steps.length >= MAX_STEPS}>
                <Plus className="h-4 w-4" /> Добавить текущую точку
              </Button>
              <Button variant="ghost" onClick={clearAll} disabled={steps.length === 0} className="ml-auto">
                <Trash2 className="h-4 w-4" /> Очистить всё
              </Button>
            </div>
          )}
          {recordError && <p className="mt-2 text-xs text-destructive">{recordError}</p>}
        </div>

        <ScrollArea className="flex-1 px-5 pb-5">
          {steps.length === 0 ? (
            <div className="flex h-32 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">
              Шагов нет. Нажми «Начать запись» и тыкай <span className="mx-1 font-mono text-foreground">{RECORD_HOTKEY}</span> в нужных местах экрана.
            </div>
          ) : (
            <div className="space-y-2">
              {steps.map((s, i) => (
                <div key={i} className="flex items-center gap-3 rounded-lg border bg-card/50 p-3">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-sm font-bold text-primary">
                    {i + 1}
                  </div>
                  <div className="flex flex-1 items-center gap-3">
                    <span className="font-mono text-sm tabular-nums">
                      ({s.x}, {s.y})
                    </span>
                    <Select value={s.button || 'left'} onValueChange={(v) => updateStep(i, { button: v })}>
                      <SelectTrigger className="h-7 w-24 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {BUTTON_OPTIONS.map((b) => (
                          <SelectItem key={b.id} value={b.id}>{b.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8 text-destructive hover:bg-destructive/10"
                    onClick={() => removeStep(i)}
                    title="Удалить шаг"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))}
              {steps.length >= MAX_STEPS && (
                <p className="pt-2 text-center text-[11px] text-muted-foreground">
                  Достигнут максимум — {MAX_STEPS} шагов. Удали лишние, чтобы записать новые.
                </p>
              )}
            </div>
          )}
        </ScrollArea>
      </Card>

      <Card className="self-start">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Repeat className="h-5 w-5 text-primary" /> Параметры
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="ms">Задержка между шагами, мс</Label>
            <Input
              id="ms"
              type="number"
              min="0"
              step="10"
              value={seq.interval_ms}
              onChange={(e) => setIntervalMs(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="lp">Повторы (0 = ∞)</Label>
            <Input
              id="lp"
              type="number"
              min="0"
              value={seq.loops ?? 0}
              onChange={(e) => setLoops(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label className="flex items-center gap-1.5"><Keyboard className="h-3.5 w-3.5" /> Хоткей пуск/стоп</Label>
            <HotkeyRecorder value={seq.hotkey ?? ''} onChange={setHotkey} placeholder="—" />
          </div>

          {running ? (
            <Button variant="destructive" className="w-full" onClick={Stop}>
              <Square className="h-4 w-4" /> Остановить
            </Button>
          ) : (
            <>
              <Button variant="neon" className="w-full" onClick={start} disabled={steps.length === 0}>
                <Play className="h-4 w-4" /> Запустить
              </Button>
              <Button variant="outline" className="w-full" onClick={() => StartSequenceDry()} disabled={steps.length === 0}>
                <Play className="h-4 w-4" /> Тест без кликов
              </Button>
            </>
          )}

          <div className="rounded-md border border-dashed bg-muted/20 p-3 text-[11px] text-muted-foreground">
            <b className="text-foreground">Как пользоваться:</b><br />
            1. Жми «Начать запись» <br />
            2. Наведи курсор на нужное место → <span className="font-mono">{RECORD_HOTKEY}</span> <br />
            3. Повтори до 10 раз (или Esc для остановки) <br />
            4. (опц.) Поменяй кнопку у каждого шага <br />
            5. Жми «Запустить»
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
