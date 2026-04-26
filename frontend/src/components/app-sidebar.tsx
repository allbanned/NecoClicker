import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import {
  Home, Layers3, FlaskConical, Palette, Settings as Cog,
  Timer as TimerIcon, Shuffle, Pin, PinOff, Footprints, Pause as PauseIcon,
} from 'lucide-react'
import { useEngine } from '@/components/engine-provider'
import { useConfig } from '@/components/config-provider'
import { SetAlwaysOnTop } from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import necoUrl from '@/assets/neco.png'

export type PageId = 'home' | 'macros' | 'steps' | 'timer' | 'jitter' | 'sandbox' | 'themes' | 'settings'

const NAV: { id: PageId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'home',     label: 'Главная',   icon: Home },
  { id: 'macros',   label: 'Макросы',   icon: Layers3 },
  { id: 'steps',    label: 'По точкам', icon: Footprints },
  { id: 'timer',    label: 'Таймер',    icon: TimerIcon },
  { id: 'jitter',   label: 'Хаос',      icon: Shuffle },
  { id: 'sandbox',  label: 'Тест',      icon: FlaskConical },
  { id: 'themes',   label: 'Темы',      icon: Palette },
  { id: 'settings', label: 'Настройки', icon: Cog },
]

export function AppSidebar({ page, setPage }: { page: PageId; setPage: (p: PageId) => void }) {
  const { running } = useEngine()
  const { cfg, reload } = useConfig()
  const onTop = !!cfg?.always_on_top

  const [paused, setPaused] = useState(false)
  useEffect(() => {
    EventsOn('engine:paused', (v: boolean) => setPaused(!!v))
    return () => EventsOff('engine:paused')
  }, [])
  // Сбрасываем paused когда движок останавливается
  useEffect(() => { if (!running) setPaused(false) }, [running])

  const togglePin = async () => {
    await SetAlwaysOnTop(!onTop)
    reload()
  }

  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r border-border bg-card/50 backdrop-blur">
      <div className="flex items-center gap-3 px-4 pb-3 pt-5">
        <div className={cn(
          'relative h-12 w-12 shrink-0 overflow-hidden rounded-xl border border-border bg-background',
          running && !paused && 'animate-pulse-glow'
        )}>
          <img src={necoUrl} alt="Neco" className="h-full w-full object-cover" />
        </div>
        <div className="flex flex-col">
          <span className="text-base font-bold tracking-tight text-glow">NecoClicker</span>
          <span className="text-[10px] uppercase tracking-widest text-muted-foreground">v1.6.4</span>
        </div>
      </div>

      <nav className="mt-2 flex flex-col gap-0.5 px-2">
        {NAV.map((n) => {
          const Icon = n.icon
          const active = page === n.id
          return (
            <button
              key={n.id}
              onClick={() => setPage(n.id)}
              className={cn(
                'group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                active
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground',
              )}
            >
              {active && (
                <span className="absolute inset-y-1 left-0 w-1 rounded-r-full bg-primary glow-primary" />
              )}
              <Icon className={cn('h-4 w-4', active && 'text-primary')} />
              {n.label}
            </button>
          )
        })}
      </nav>

      <div className="mt-auto space-y-2 px-3 py-4">
        <button
          onClick={togglePin}
          className={cn(
            'flex w-full items-center justify-between gap-2 rounded-lg border bg-background/60 px-3 py-2 text-xs transition-all',
            onTop ? 'border-primary/50 text-primary glow-primary' : 'border-border text-muted-foreground hover:border-primary/30',
          )}
          title={onTop ? 'Убрать «поверх всех окон»' : 'Закрепить поверх всех окон'}
        >
          {onTop ? <Pin className="h-3.5 w-3.5" /> : <PinOff className="h-3.5 w-3.5" />}
          <span className="flex-1 text-left">Поверх окон</span>
          <span className={cn('text-[10px] font-bold', onTop && 'text-primary')}>{onTop ? 'ON' : 'OFF'}</span>
        </button>

        <div className={cn(
          'flex items-center gap-2 rounded-lg border border-border bg-background/60 px-3 py-2 text-xs',
          paused && 'border-yellow-500/60',
          running && !paused && 'border-primary/50',
        )}>
          {paused ? (
            <PauseIcon className="h-3 w-3 text-yellow-500" />
          ) : (
            <span className={cn(
              'h-2 w-2 rounded-full',
              running ? 'bg-primary glow-primary animate-pulse' : 'bg-muted-foreground/40'
            )} />
          )}
          <span className={cn(
            paused ? 'text-yellow-500 font-semibold'
              : running ? 'text-primary font-semibold' : 'text-muted-foreground'
          )}>
            {paused ? 'Пауза' : running ? 'Активен' : 'Остановлен'}
          </span>
        </div>
      </div>
    </aside>
  )
}
