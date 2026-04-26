import * as React from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { IsRunning } from '../../wailsjs/go/main/App'

export type CPSReport = { cps: number; total: number }

type EngineCtx = {
  running: boolean
  paused: boolean
  log: string[]
  cps: CPSReport
  history: number[]
  clearLog: () => void
}

const Ctx = React.createContext<EngineCtx>({
  running: false,
  paused: false,
  log: [],
  cps: { cps: 0, total: 0 },
  history: [],
  clearLog: () => {},
})

/**
 * EngineProvider подписывается на события движка ОДИН РАЗ на корне приложения.
 * Wails runtime.EventsOff(eventName) удаляет ВСЕ хендлеры для имени, поэтому
 * иметь несколько копий useEngine() = выстрел в ногу. Контекст — единственный
 * безопасный способ.
 */
export function EngineProvider({ children }: { children: React.ReactNode }) {
  const [running, setRunning] = React.useState(false)
  const [paused, setPaused] = React.useState(false)
  const [log, setLog] = React.useState<string[]>([])
  const [cps, setCps] = React.useState<CPSReport>({ cps: 0, total: 0 })
  const [history, setHistory] = React.useState<number[]>([])

  React.useEffect(() => {
    let mounted = true
    IsRunning().then((r) => { if (mounted) setRunning(r) })

    EventsOn('engine:state', (r: boolean) => {
      setRunning(r)
      if (!r) setPaused(false) // остановка движка снимает паузу
    })
    EventsOn('engine:paused', (p: boolean) => setPaused(!!p))
    EventsOn('engine:log', (line: string) => {
      setLog((prev) => {
        const next = prev.length > 800 ? prev.slice(-700) : prev.slice()
        next.push(line)
        return next
      })
    })
    EventsOn('engine:cps', (r: CPSReport) => {
      setCps(r)
      setHistory((prev) => {
        const next = prev.slice()
        next.push(r.cps)
        if (next.length > 80) next.shift()
        return next
      })
    })

    return () => {
      mounted = false
      EventsOff('engine:state', 'engine:paused', 'engine:log', 'engine:cps')
    }
  }, [])

  const clearLog = React.useCallback(() => setLog([]), [])

  const value = React.useMemo(
    () => ({ running, paused, log, cps, history, clearLog }),
    [running, paused, log, cps, history, clearLog],
  )
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useEngine() {
  return React.useContext(Ctx)
}
