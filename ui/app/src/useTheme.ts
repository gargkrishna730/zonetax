import { useEffect, useState } from 'react'
import type { Theme } from './theme'

const KEY = 'zonetax-theme'

/** Theme follows the OS (prefers-color-scheme) until the user picks one; the choice persists. */
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(() => {
    try {
      const saved = localStorage.getItem(KEY)
      if (saved === 'dark' || saved === 'light') return saved
    } catch {
      /* storage unavailable */
    }
    return typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
  }, [theme])
  const setTheme = (t: Theme) => {
    setThemeState(t)
    try {
      localStorage.setItem(KEY, t)
    } catch {
      /* ignore */
    }
  }
  return [theme, setTheme]
}
