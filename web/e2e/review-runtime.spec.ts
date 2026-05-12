import { expect, test } from '@playwright/test'

const REVIEW_NAME = process.env.REVIEW_PROJECT_NAME ?? 'RV Runtime Project'
const REVIEW_GOAL = process.env.REVIEW_PROJECT_GOAL ?? 'Validate full runtime review flow'
const REVIEW_DESCRIPTION =
  process.env.REVIEW_PROJECT_DESCRIPTION ?? 'Browser-created project for review runtime evidence'

test.describe('review runtime project creation', () => {
  test('creates a project through the real web console', async ({ page }) => {
    await page.goto('/')
    await page.waitForURL(/\/projects(\?|$)/, { timeout: 15_000 })

    await page.getByRole('button', { name: '新建项目' }).first().click()
    await page.getByLabel('项目名称').fill(REVIEW_NAME)
    await page.getByLabel('目标 (Goal)').fill(REVIEW_GOAL)
    await page.getByLabel('描述（可选）').fill(REVIEW_DESCRIPTION)
    await page.getByRole('button', { name: '创建项目' }).click()

    await expect(page.getByRole('heading', { name: '项目已创建' })).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByText('aitask init --project')).toBeVisible({ timeout: 15_000 })

    const commandText =
      (await page
        .locator('code')
        .filter({ hasText: 'aitask init --project' })
        .first()
        .textContent()) ?? ''
    expect(commandText).toContain('aitask init --project')

    await page.getByRole('button', { name: /前往任务看板|继续|进入项目/ }).click()
    await page.waitForURL(/\/projects\/prj_[A-Za-z0-9]+\/tasks/, { timeout: 15_000 })
  })
})
