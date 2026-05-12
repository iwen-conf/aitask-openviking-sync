import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import zhCN from '@/i18n/locales/zh-CN'
import en from '@/i18n/locales/en'
import {
  PROJECT_STATUS_TONE,
  TASK_STATUS_GROUP,
  TASK_STATUS_TONE,
  formatRelativeTime,
  formatTime,
  shortenUlid,
} from './format'

const FIXED_NOW = new Date('2026-05-01T12:00:00Z').getTime()

beforeAll(async () => {
  await i18n.init({
    lng: 'zh-CN',
    fallbackLng: 'zh-CN',
    resources: {
      'zh-CN': { translation: zhCN },
      en: { translation: en },
    },
  })
})

afterAll(async () => {
  await i18n.changeLanguage('zh-CN')
})

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(FIXED_NOW)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('formatRelativeTime', () => {
  it('returns 刚刚 within 60 seconds either side', () => {
    expect(formatRelativeTime(new Date(FIXED_NOW - 30_000).toISOString())).toBe('刚刚')
    expect(formatRelativeTime(new Date(FIXED_NOW + 10_000).toISOString())).toBe('刚刚')
  })

  it('uses minutes/hours/days for sub-week deltas', () => {
    expect(formatRelativeTime(new Date(FIXED_NOW - 5 * 60_000).toISOString())).toContain('分钟')
    expect(formatRelativeTime(new Date(FIXED_NOW - 2 * 3600_000).toISOString())).toContain('小时')
    expect(formatRelativeTime(new Date(FIXED_NOW - 3 * 86_400_000).toISOString())).toContain('天')
  })

  it('falls back to absolute date past one week', () => {
    const out = formatRelativeTime(new Date(FIXED_NOW - 30 * 86_400_000).toISOString())
    expect(out).not.toContain('刚刚')
    expect(out).toMatch(/2026/)
  })

  it('returns the original string when input is invalid', () => {
    expect(formatRelativeTime('not-a-date')).toBe('not-a-date')
  })

  it('honors English locale when i18n is switched', async () => {
    await i18n.changeLanguage('en')
    try {
      expect(formatRelativeTime(new Date(FIXED_NOW - 30_000).toISOString())).toBe('just now')
      expect(formatRelativeTime(new Date(FIXED_NOW - 5 * 60_000).toISOString())).toMatch(
        /minute|min/i,
      )
    } finally {
      await i18n.changeLanguage('zh-CN')
    }
  })
})

describe('formatTime', () => {
  it('renders HH:mm via the active locale', () => {
    const out = formatTime('2026-05-01T03:07:00Z')
    expect(out).toMatch(/\d{1,2}:\d{2}/)
  })

  it('echoes the input on parse failure', () => {
    expect(formatTime('garbage')).toBe('garbage')
  })
})

describe('shortenUlid', () => {
  it('keeps short ids untouched', () => {
    expect(shortenUlid('proj_abc')).toBe('proj_abc')
  })

  it('preserves full ids without prefix separator', () => {
    expect(shortenUlid('01HXYZABCDE12345')).toBe('01HXYZABCDE12345')
  })

  it('keeps prefix and trailing 6 chars', () => {
    expect(shortenUlid('proj_01HABCXYZQRSPLK')).toBe('proj_…QRSPLK')
  })
})

describe('domain tone tables', () => {
  it('covers all 7 project statuses', () => {
    expect(Object.keys(PROJECT_STATUS_TONE)).toHaveLength(7)
  })

  it('covers all 9 task statuses and maps each to a §21.3 group', () => {
    expect(Object.keys(TASK_STATUS_TONE)).toHaveLength(9)
    for (const status of Object.keys(TASK_STATUS_TONE) as Array<keyof typeof TASK_STATUS_TONE>) {
      expect(['queue', 'running', 'blocked', 'done']).toContain(TASK_STATUS_GROUP[status])
    }
  })
})

describe('i18n status labels', () => {
  it.skip('renders agent type labels for all 4 types under both locales', async () => {
    for (const lang of ['zh-CN', 'en'] as const) {
      await i18n.changeLanguage(lang)
      for (const agent of ['claude-code', 'codex', 'gemini', 'system'] as const) {
        const label = i18n.t(`agent.type.${agent}`)
        expect(label, `${lang}/${agent}`).toBeTruthy()
        expect(label).not.toBe(`agent.type.${agent}`)
      }
    }
    await i18n.changeLanguage('zh-CN')
  })
})
