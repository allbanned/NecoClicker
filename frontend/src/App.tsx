import { useEffect, useState } from 'react'
import { ThemeProvider, type ThemeId, THEMES, detectSystemTheme } from '@/components/theme-provider'
import { EngineProvider } from '@/components/engine-provider'
import { ConfirmProvider } from '@/components/confirm-dialog'
import { AppSidebar, type PageId } from '@/components/app-sidebar'
import { MiniBar } from '@/components/mini-bar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { HomePage } from '@/pages/home'
import { MacrosPage } from '@/pages/macros'
import { StepsPage } from '@/pages/steps'
import { TimerPage } from '@/pages/timer'
import { JitterPage } from '@/pages/jitter'
import { SandboxPage } from '@/pages/sandbox'
import { ThemesPage } from '@/pages/themes'
import { SettingsPage } from '@/pages/settings'
import { GetConfig, SetTheme } from '../wailsjs/go/main/App'
import { useConfig } from '@/hooks/use-config'

function FullShell() {
  const [page, setPage] = useState<PageId>('home')
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <AppSidebar page={page} setPage={setPage} />
      <main className="flex-1 overflow-y-auto p-6">
        <header className="mb-5">
          <h1 className="text-xl font-bold tracking-tight">{labelFor(page)}</h1>
        </header>
        {page === 'home' && <HomePage />}
        {page === 'macros' && <MacrosPage />}
        {page === 'steps' && <StepsPage />}
        {page === 'timer' && <TimerPage />}
        {page === 'jitter' && <JitterPage />}
        {page === 'sandbox' && <SandboxPage />}
        {page === 'themes' && <ThemesPage />}
        {page === 'settings' && <SettingsPage />}
      </main>
    </div>
  )
}

function ShellSwitch() {
  const { cfg } = useConfig()
  if (cfg?.always_on_top) return <MiniBar />
  return <FullShell />
}

function Shell({ initialTheme }: { initialTheme: ThemeId }) {
  return (
    <ThemeProvider initial={initialTheme}>
      <EngineProvider>
       <ConfirmProvider>
        <TooltipProvider delayDuration={300}>
         <ShellSwitch />
        </TooltipProvider>
       </ConfirmProvider>
      </EngineProvider>
    </ThemeProvider>
  )
}

function labelFor(p: PageId): string {
  switch (p) {
    case 'home': return 'Главная'
    case 'macros': return 'Редактор макросов'
    case 'steps': return 'Пошаговый кликер'
    case 'timer': return 'Кликер с таймером'
    case 'jitter': return 'Хаотичный кликер'
    case 'sandbox': return 'Тест и логи'
    case 'themes': return 'Темы'
    case 'settings': return 'Настройки'
  }
}

export default function App() {
  const [theme, setTheme] = useState<ThemeId | null>(null)

  useEffect(() => {
    GetConfig()
      .then(async (c) => {
        const stored = (c?.theme || '') as string
        if (!stored) {
          const sys = detectSystemTheme()
          setTheme(sys)
          try { await SetTheme(sys) } catch {}
          return
        }
        const valid = THEMES.find((x) => x.id === stored as ThemeId)
        setTheme(valid ? (stored as ThemeId) : 'light')
      })
      .catch(() => setTheme(detectSystemTheme()))
  }, [])

  if (!theme) return null
  return <Shell initialTheme={theme} />
}
