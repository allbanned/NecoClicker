import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Plus, Trash2, ChevronUp, ChevronDown, Play, Square, Crosshair, MousePointerClick, Move, Timer, Star, MousePointer2 } from 'lucide-react'
import { useConfig } from '@/hooks/use-config'
import { useEngine } from '@/components/engine-provider'
import { useConfirm } from '@/components/confirm-dialog'
import { HotkeyRecorder } from '@/components/hotkey-recorder'
import { CursorPos, StartChain, Stop, SetActiveChain } from '../../wailsjs/go/main/App'
import { macro } from '../../wailsjs/go/models'
import { cn } from '@/lib/utils'

type Action = macro.Action
type ActionKind = 'click' | 'delay' | 'move' | 'drag'

function newAction(type: ActionKind): Action {
  const a = new macro.Action({ type })
  if (type === 'click') {
    a.button = 'left'
    a.use_current = true
  }
  if (type === 'delay') a.delay_ms = 100
  if (type === 'drag') {
    a.button = 'left'
    a.duration_ms = 300
  }
  return a
}

export function MacrosPage() {
  const { cfg, saveChain, deleteChain, reload } = useConfig()
  const { running } = useEngine()
  const { ask } = useConfirm()
  const [selected, setSelected] = useState(0)

  const chains = cfg?.chains ?? []
  const activeChain = cfg?.active_chain ?? 0
  const cur = chains[selected]

  const [name, setName] = useState('')
  const [hotkey, setHotkey] = useState('')
  const [loops, setLoops] = useState('0')
  const [actions, setActions] = useState<Action[]>([])

  useEffect(() => {
    if (!cur) {
      setName(''); setHotkey(''); setLoops('0'); setActions([])
      return
    }
    setName(cur.name)
    setHotkey(cur.hotkey ?? '')
    setLoops(String(cur.loops ?? 0))
    setActions(cur.actions ?? [])
  }, [selected, cfg])

  const addChain = async () => {
    const ch = new macro.Chain({ name: `Chain ${chains.length + 1}`, loops: 1, actions: [] })
    await saveChain(-1, ch)
    setSelected(chains.length)
  }

  const persist = async (overrides: Partial<{ name: string; hotkey: string; loops: number; actions: Action[] }> = {}, acts = actions) => {
    if (selected < 0) return
    const ch = new macro.Chain({
      name: overrides.name ?? (name || `Chain ${selected + 1}`),
      hotkey: overrides.hotkey ?? hotkey,
      loops: overrides.loops ?? (parseInt(loops) || 0),
      actions: overrides.actions ?? acts,
    })
    await saveChain(selected, ch)
  }

  const removeChain = async () => {
    if (!cur) return
    const ok = await ask({
      title: 'Удалить цепочку?',
      description: `Цепочка "${cur.name}" со всеми шагами будет безвозвратно удалена.`,
      confirmText: 'Удалить',
      destructive: true,
    })
    if (!ok) return
    await deleteChain(selected)
    setSelected(Math.max(0, selected - 1))
  }

  const updateAction = (i: number, patch: Partial<Action>) => {
    setActions((prev) => {
      const next = prev.slice()
      next[i] = new macro.Action({ ...next[i], ...patch } as any)
      persist({}, next)
      return next
    })
  }
  const moveAction = (i: number, dir: -1 | 1) => {
    setActions((prev) => {
      const j = i + dir
      if (j < 0 || j >= prev.length) return prev
      const next = prev.slice()
      ;[next[i], next[j]] = [next[j], next[i]]
      persist({}, next)
      return next
    })
  }
  const removeAction = (i: number) => {
    setActions((prev) => {
      const next = prev.filter((_, k) => k !== i)
      persist({}, next)
      return next
    })
  }
  const addAction = (type: ActionKind) => {
    setActions((prev) => {
      const next = [...prev, newAction(type)]
      persist({}, next)
      return next
    })
  }

  const runChain = async () => {
    await persist()
    await StartChain(selected)
  }

  return (
    <div className="grid h-[calc(100vh-9rem)] gap-4 lg:grid-cols-[280px_1fr]">
      <Card className="flex flex-col overflow-hidden">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center justify-between">
            <span>Цепочки</span>
            <Button size="sm" variant="ghost" onClick={addChain}><Plus className="h-4 w-4" /></Button>
          </CardTitle>
        </CardHeader>
        <ScrollArea className="flex-1">
          <div className="flex flex-col gap-1 px-3 pb-3">
            {chains.map((c, i) => (
              <button
                key={i}
                onClick={() => setSelected(i)}
                className={cn(
                  'group flex flex-col items-start gap-0.5 rounded-md border border-transparent px-3 py-2 text-left text-sm transition-colors',
                  selected === i ? 'border-primary/60 bg-primary/10 text-primary' : 'hover:bg-accent',
                )}
              >
                <span className="flex items-center gap-1.5 font-medium">
                  {i === activeChain && <Star className="h-3 w-3 fill-current text-primary" />}
                  {c.name || `Chain ${i + 1}`}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {c.actions?.length ?? 0} шагов{c.hotkey ? ` · ${c.hotkey}` : ''}
                </span>
              </button>
            ))}
            {chains.length === 0 && (
              <div className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">
                Нет цепочек. Нажми <span className="text-foreground">+</span> чтобы создать.
              </div>
            )}
          </div>
        </ScrollArea>
      </Card>

      {cur ? (
        <Card className="flex flex-col overflow-hidden">
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between gap-3">
              <div className="grid flex-1 gap-3 sm:grid-cols-3">
                <div className="space-y-1">
                  <Label>Имя</Label>
                  <Input value={name} onChange={(e) => setName(e.target.value)} onBlur={() => persist()} />
                </div>
                <div className="space-y-1">
                  <Label>Хоткей</Label>
                  <HotkeyRecorder value={hotkey} onChange={(v) => { setHotkey(v); persist({ hotkey: v }) }} placeholder="—" />
                </div>
                <div className="space-y-1">
                  <Label>Повторы (0 = ∞)</Label>
                  <Input type="number" min="0" value={loops} onChange={(e) => setLoops(e.target.value)} onBlur={() => persist()} />
                </div>
              </div>
              <div className="flex items-end gap-2">
                {running ? (
                  <Button variant="destructive" onClick={Stop}><Square className="h-4 w-4" /> Стоп</Button>
                ) : (
                  <Button variant="neon" onClick={runChain}><Play className="h-4 w-4" /> Запустить</Button>
                )}
                <Button
                  variant={selected === activeChain ? 'default' : 'ghost'}
                  size="icon"
                  onClick={async () => { await SetActiveChain(selected); reload() }}
                  title={selected === activeChain ? 'Активная цепочка' : 'Сделать активной'}
                  disabled={selected === activeChain}
                >
                  <Star className={cn('h-4 w-4', selected === activeChain && 'fill-current')} />
                </Button>
                <Button variant="ghost" size="icon" onClick={removeChain} title="Удалить"><Trash2 className="h-4 w-4" /></Button>
              </div>
            </div>
          </CardHeader>

          <div className="flex items-center gap-2 px-5 pb-3">
            <Button size="sm" variant="outline" onClick={() => addAction('click')}><MousePointerClick className="h-3.5 w-3.5" /> Клик</Button>
            <Button size="sm" variant="outline" onClick={() => addAction('delay')}><Timer className="h-3.5 w-3.5" /> Задержка</Button>
            <Button size="sm" variant="outline" onClick={() => addAction('move')}><Move className="h-3.5 w-3.5" /> Перемещение</Button>
            <Button size="sm" variant="outline" onClick={() => addAction('drag')}><MousePointer2 className="h-3.5 w-3.5" /> Drag</Button>
          </div>

          <ScrollArea className="flex-1 px-5 pb-5">
            <div className="space-y-2">
              {actions.map((a, i) => (
                <ActionRow
                  key={i}
                  index={i}
                  action={a}
                  onChange={(p) => updateAction(i, p)}
                  onMove={(d) => moveAction(i, d)}
                  onRemove={() => removeAction(i)}
                  onCapture={async () => {
                    const [cx, cy] = await CursorPos()
                    updateAction(i, { x: cx, y: cy } as any)
                  }}
                />
              ))}
              {actions.length === 0 && (
                <div className="rounded-md border border-dashed p-6 text-center text-xs text-muted-foreground">
                  Добавь первый шаг кнопками выше.
                </div>
              )}
            </div>
          </ScrollArea>
        </Card>
      ) : (
        <Card className="flex items-center justify-center text-sm text-muted-foreground">
          Выбери или создай цепочку.
        </Card>
      )}
    </div>
  )
}

function ActionRow({ index, action, onChange, onMove, onRemove, onCapture }: {
  index: number
  action: Action
  onChange: (p: Partial<Action>) => void
  onMove: (dir: -1 | 1) => void
  onRemove: () => void
  onCapture: () => void
}) {
  const Icon = action.type === 'click' ? MousePointerClick : action.type === 'delay' ? Timer : Move

  return (
    <div className="rounded-lg border border-border bg-card/50 p-3">
      <div className="mb-2 flex items-center justify-between">
        <div className="flex items-center gap-2 text-xs font-semibold">
          <span className="flex h-6 w-6 items-center justify-center rounded-md bg-primary/10 text-primary">
            <Icon className="h-3.5 w-3.5" />
          </span>
          <span>#{index + 1}</span>
          <span className="uppercase tracking-wider text-muted-foreground">{action.type}</span>
        </div>
        <div className="flex items-center gap-1">
          <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => onMove(-1)}><ChevronUp className="h-3.5 w-3.5" /></Button>
          <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => onMove(1)}><ChevronDown className="h-3.5 w-3.5" /></Button>
          <Button size="icon" variant="ghost" className="h-7 w-7 text-destructive hover:bg-destructive/10" onClick={onRemove}><Trash2 className="h-3.5 w-3.5" /></Button>
        </div>
      </div>

      {action.type === 'delay' && (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="space-y-1">
            <Label>Задержка (мс)</Label>
            <Input type="number" min="0" value={action.delay_ms ?? 0} onChange={(e) => onChange({ delay_ms: parseInt(e.target.value) || 0 })} />
          </div>
        </div>
      )}

      {action.type === 'click' && (
        <div className="grid gap-2 sm:grid-cols-3">
          <div className="space-y-1">
            <Label>Кнопка</Label>
            <Select value={action.button || 'left'} onValueChange={(v) => onChange({ button: v })}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="left">ЛКМ</SelectItem>
                <SelectItem value="right">ПКМ</SelectItem>
                <SelectItem value="middle">СКМ</SelectItem>
                <SelectItem value="x1">X1 (Mouse4)</SelectItem>
                <SelectItem value="x2">X2 (Mouse5)</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>X</Label>
            <Input type="number" value={action.x ?? 0} onChange={(e) => onChange({ x: parseInt(e.target.value) || 0 })} disabled={!!action.use_current} />
          </div>
          <div className="space-y-1">
            <Label>Y</Label>
            <Input type="number" value={action.y ?? 0} onChange={(e) => onChange({ y: parseInt(e.target.value) || 0 })} disabled={!!action.use_current} />
          </div>
          <div className="col-span-full flex flex-wrap items-center gap-3 pt-1">
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={!!action.use_current} onCheckedChange={(v) => onChange({ use_current: v })} />
              По текущему курсору
            </label>
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={!!action.relative} onCheckedChange={(v) => onChange({ relative: v })} />
              Относительно курсора
            </label>
            <Button size="sm" variant="outline" onClick={onCapture}><Crosshair className="h-3.5 w-3.5" /> Захватить</Button>
          </div>
        </div>
      )}

      {action.type === 'move' && (
        <div className="grid gap-2 sm:grid-cols-3">
          <div className="space-y-1">
            <Label>X</Label>
            <Input type="number" value={action.x ?? 0} onChange={(e) => onChange({ x: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="space-y-1">
            <Label>Y</Label>
            <Input type="number" value={action.y ?? 0} onChange={(e) => onChange({ y: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="flex items-end">
            <Button size="sm" variant="outline" onClick={onCapture} className="w-full"><Crosshair className="h-3.5 w-3.5" /> Захватить</Button>
          </div>
          <div className="col-span-full">
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={!!action.relative} onCheckedChange={(v) => onChange({ relative: v })} />
              Относительно курсора
            </label>
          </div>
        </div>
      )}

      {action.type === 'drag' && (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="space-y-1">
            <Label>Кнопка</Label>
            <Select value={action.button || 'left'} onValueChange={(v) => onChange({ button: v })}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="left">ЛКМ</SelectItem>
                <SelectItem value="right">ПКМ</SelectItem>
                <SelectItem value="middle">СКМ</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>Длительность (мс)</Label>
            <Input type="number" min="0" value={action.duration_ms ?? 0} onChange={(e) => onChange({ duration_ms: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="space-y-1">
            <Label>Старт X</Label>
            <Input type="number" value={action.x ?? 0} onChange={(e) => onChange({ x: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="space-y-1">
            <Label>Старт Y</Label>
            <Input type="number" value={action.y ?? 0} onChange={(e) => onChange({ y: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="space-y-1">
            <Label>Конец X</Label>
            <Input type="number" value={action.end_x ?? 0} onChange={(e) => onChange({ end_x: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="space-y-1">
            <Label>Конец Y</Label>
            <Input type="number" value={action.end_y ?? 0} onChange={(e) => onChange({ end_y: parseInt(e.target.value) || 0 })} />
          </div>
          <div className="col-span-full flex flex-wrap items-center gap-2">
            <Button size="sm" variant="outline" onClick={async () => {
              const { CursorPos } = await import('../../wailsjs/go/main/App')
              const [x, y] = await CursorPos()
              onChange({ x, y } as any)
            }}><Crosshair className="h-3.5 w-3.5" /> Захватить старт</Button>
            <Button size="sm" variant="outline" onClick={async () => {
              const { CursorPos } = await import('../../wailsjs/go/main/App')
              const [x, y] = await CursorPos()
              onChange({ end_x: x, end_y: y } as any)
            }}><Crosshair className="h-3.5 w-3.5" /> Захватить конец</Button>
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={!!action.relative} onCheckedChange={(v) => onChange({ relative: v })} />
              Относительно курсора
            </label>
          </div>
        </div>
      )}
    </div>
  )
}
