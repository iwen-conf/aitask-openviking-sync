import { useTranslation } from 'react-i18next'
import { Languages } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SUPPORTED_LANGUAGES, type SupportedLanguage } from '@/i18n'

/**
 * 语言切换器。点击后立即生效，无需刷新；选择记忆于 localStorage `aitask.lang`。
 */
export function LanguageSwitcher() {
  const { t, i18n } = useTranslation()
  const current = (i18n.resolvedLanguage ?? i18n.language ?? 'zh-CN') as SupportedLanguage

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-2.5 text-xs font-medium text-[hsl(var(--muted-foreground))] transition-colors hover:border-[hsl(var(--primary)/0.4)] hover:text-[hsl(var(--foreground))]"
          aria-label={t('language.label')}
        >
          <Languages className="h-3.5 w-3.5" />
          <span>{t(`language.${current}`)}</span>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuLabel>{t('language.label')}</DropdownMenuLabel>
        {SUPPORTED_LANGUAGES.map((lang) => (
          <DropdownMenuItem
            key={lang}
            selected={current === lang}
            onSelect={() => {
              if (lang !== current) void i18n.changeLanguage(lang)
            }}
          >
            <span>{t(`language.${lang}`)}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
