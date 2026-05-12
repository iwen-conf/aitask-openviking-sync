import { expect, test } from '@playwright/test'

test.describe('home smoke', () => {
  test('renders the console shell and reaches /projects', async ({ page }) => {
    await page.goto('/')

    // 根路由会 navigate 到 /projects，等待 URL 落定。
    await page.waitForURL(/\/projects(\?|$)/, { timeout: 10_000 })

    await expect(page).toHaveTitle(/AgentFlow/i)

    // Sidebar 与 Topbar 应当被渲染（具体文案后续可由 i18n key 替换）。
    await expect(page.locator('header').first()).toBeVisible()
    await expect(page.locator('nav').first()).toBeVisible()
  })

  test('runtime config script is loaded before main bundle', async ({ page }) => {
    await page.goto('/')
    const runtimeConfig = await page.evaluate(() => {
      return (window as unknown as { __RUNTIME_CONFIG__?: unknown }).__RUNTIME_CONFIG__
    })
    expect(runtimeConfig).toBeTruthy()
  })

  test('language can switch between zh-CN and en without reload', async ({ page, context }) => {
    await context.clearCookies()
    await page.addInitScript(() => {
      try {
        window.localStorage.clear()
      } catch {
        /* ignore */
      }
    })
    await page.goto('/')
    await page.waitForURL(/\/projects(\?|$)/, { timeout: 10_000 })

    // Playwright 默认 navigator.language=en-US,convertDetectedLanguage 映射为 'en'。
    const enSwitcher = page.locator('button[aria-label="Language"]')
    await expect(enSwitcher).toBeVisible()
    await enSwitcher.click()

    // 切换到中文,断言 aria-label 立即变化(无 reload)。
    await page.getByRole('menuitem', { name: '简体中文' }).click()
    await expect(page.locator('button[aria-label="语言"]')).toBeVisible()
  })
})
