/**
 * 主题管理 Hook
 * 负责：初始化应用主题、监听系统主题变化（system 模式下）
 * Semi Design 深色模式通过 body[theme-mode="dark"] 实现
 */
import { useEffect } from 'react'
import { applyThemeToDOM, useAppStore } from '@/stores/app'
import { THEME_MODES, type ThemeMode } from '@/config/constants'

export function useTheme() {
  const themeMode = useAppStore((s) => s.themeMode)
  const setThemeMode = useAppStore((s) => s.setThemeMode)

  // 初始挂载时应用一次主题（刷新页面场景）
  useEffect(() => {
    applyThemeToDOM(themeMode)
  }, [themeMode])

  // system 模式下监听操作系统主题变化
  useEffect(() => {
    if (themeMode !== THEME_MODES.system) return
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const listener = () => applyThemeToDOM(THEME_MODES.system)
    media.addEventListener('change', listener)
    return () => media.removeEventListener('change', listener)
  }, [themeMode])

  const isDark =
    themeMode === THEME_MODES.dark ||
    (themeMode === THEME_MODES.system && window.matchMedia('(prefers-color-scheme: dark)').matches)

  const toggleTheme = () => {
    setThemeMode(isDark ? THEME_MODES.light : THEME_MODES.dark)
  }

  return {
    themeMode,
    isDark,
    setThemeMode: (mode: ThemeMode) => setThemeMode(mode),
    toggleTheme,
  }
}
