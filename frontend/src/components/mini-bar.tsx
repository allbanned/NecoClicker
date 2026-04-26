import { useEngine } from '@/components/engine-provider'
import { useConfig } from '@/hooks/use-config'
import {
  StartSimple, Stop, TogglePause, SetAlwaysOnTop,
} from '../../wailsjs/go/main/App'
import { Play, Square, Pause, Maximize2 } from 'lucide-react'
import necoUrl from '@/assets/neco.png'
import { cn } from '@/lib/utils'

/**
 * MiniBar — компактный режим, активный когда Always-on-Top.
 * Размер окна фиксирован 380×100 (см. app.go: miniWindowW/H).
 *
 * Drag регулируется CSS-переменной --wails-draggable: drag,
 * чтобы пользователь мог тащить окошко за фон.
 */
export function MiniBar() {
  const { running, paused, cps } = useEngine()
  const { cfg, reload } = useConfig()
  const profile = cfg?.profiles?.[cfg?.active ?? 0]
  const profileName = profile?.name ?? 'Default'

  const onStartStop = async () => {
    if (running) await Stop()
    else await StartSimple()
  }

  const expand = async () => {
    await SetAlwaysOnTop(false)
    reload()
  }

  return (
    <div
      className="flex h-screen w-screen items-center gap-2 overflow-hidden bg-background px-3 text-foreground select-none"
      style={{ ['--wails-draggable' as any]: 'drag' } as React.CSSProperties}
    >
      <div className={cn(
        'relative h-12 w-12 shrink-0 overflow-hidden rounded-lg border border-border',
        running && 'animate-pulse-glow border-primary/60',
      )}>
        <img src={necoUrl} alt="" className="h-full w-full object-cover" />
      </div>

      <div className="flex min-w-0 flex-1 flex-col leading-tight">
        <div className="truncate text-xs font-bold uppercase tracking-wider">
          {profileName}
        </div>
        <div className={cn(
          'truncate text-[11px]',
          paused ? 'text-yellow-500' : running ? 'text-primary' : 'text-muted-foreground',
        )}>
          {paused
            ? 'Пауза'
            : running
              ? `${cps.cps.toFixed(1)} CPS · ${Number(cps.total).toLocaleString()}`
              : 'Остановлен'}
        </div>
      </div>

      {/* Кнопки — отключаем drag на них */}
      <div
        className="flex shrink-0 items-center gap-1"
        style={{ ['--wails-draggable' as any]: 'no-drag' } as React.CSSProperties}
      >
        <button
          onClick={onStartStop}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-md border transition-all',
            running
              ? 'border-destructive/50 bg-destructive text-destructive-foreground hover:bg-destructive/90'
              : 'border-primary/50 bg-primary text-primary-foreground hover:bg-primary/90 glow-primary',
          )}
          title={running ? 'Остановить' : 'Запустить активный профиль'}
        >
          {running ? <Square className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </button>
        <button
          onClick={() => TogglePause()}
          disabled={!running}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-md border bg-background transition-all',
            paused ? 'border-yellow-500/60 text-yellow-500' : 'border-border',
            !running ? 'opacity-30' : 'hover:border-primary/50 hover:bg-accent',
          )}
          title={paused ? 'Возобновить' : 'Пауза'}
        >
          {paused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
        </button>
        <button
          onClick={expand}
          className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-background transition-all hover:border-primary/50 hover:bg-accent"
          title="Развернуть полную версию"
        >
          <Maximize2 className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
