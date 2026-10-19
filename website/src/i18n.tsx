import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { detectLocale, persistLocale, SITE_META, type Locale } from './locale'

// ---------------------------------------------------------------------------
// 站点语言上下文（REQ-360）：语言态 + 切换 + 持久化 + <html>/<title>/meta 同步。
// 页面用法：const { locale, setLocale } = useLocale()；纯文案字典用 useCopy。
// ---------------------------------------------------------------------------

interface LocaleContextValue {
  locale: Locale
  setLocale: (l: Locale) => void
}

const LocaleContext = createContext<LocaleContextValue>({ locale: 'zh', setLocale: () => {} })

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(detectLocale)

  const setLocale = (l: Locale) => {
    setLocaleState(l)
    persistLocale(l)
  }

  useEffect(() => {
    const meta = SITE_META[locale]
    document.documentElement.lang = meta.htmlLang
    document.title = meta.title
    document.querySelector('meta[name="description"]')?.setAttribute('content', meta.description)
  }, [locale])

  const value = useMemo(() => ({ locale, setLocale }), [locale, setLocale])
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
}

export function useLocale(): LocaleContextValue {
  return useContext(LocaleContext)
}

/** 页面级双语文案字典：dict.zh 为事实源，dict.en 同形（调用处以 typeof zh 固化约束） */
export function useCopy<T>(dict: { zh: T; en: T }): T {
  return dict[useContext(LocaleContext).locale]
}
