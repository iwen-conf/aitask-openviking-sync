import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'
import zhCN from './locales/zh-CN'
import en from './locales/en'
import './types'

export const SUPPORTED_LANGUAGES = ['zh-CN', 'en'] as const
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number]

/**
 * 立即调用 init,resources 已内联无需远端加载,Promise 在同步代码执行后下一 microtask 解析。
 * `i18nReady` 暴露给 `main.tsx`,确保 React 在 i18n 就绪后再渲染,避免首屏出现 key fallback。
 */
export const i18nReady = i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      'zh-CN': { translation: zhCN },
      en: { translation: en },
    },
    fallbackLng: 'zh-CN',
    supportedLngs: SUPPORTED_LANGUAGES,
    interpolation: {
      // React 已对插值做转义。
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: 'aitask.lang',
      caches: ['localStorage'],
      // 把浏览器自带的 `en-US` / `zh-Hans-CN` 等映射到我们维护的两个语言代码。
      convertDetectedLanguage: (lng: string) => {
        const lower = lng.toLowerCase()
        if (lower.startsWith('zh')) return 'zh-CN'
        if (lower.startsWith('en')) return 'en'
        return lng
      },
    },
    react: {
      useSuspense: false,
    },
  })

export default i18n
