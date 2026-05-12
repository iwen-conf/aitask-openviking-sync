import { test, expect } from '@playwright/test'

const mockProject = {
  projectId: 'prj_mock',
  name: 'Mock Project',
  goal: 'Mock Goal',
  description: '',
  status: 'active',
  activeSessionId: 'sess_1',
  openvikingRoot: 'viking://aitask/projects/prj_mock',
  openvikingNamespace: 'mock-namespace',
  openvikingWorkspaceId: 'mock-workspace',
  roomId: 'room_1',
  operatorLabel: 'operator',
  completionPolicy: {
    requiredTasks: 'all_required_done',
    blockedTasks: 'none',
    failedTasks: 'none',
    reviewPolicy: 'optional',
  },
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
}

test.describe('OpenViking — system settings (credentials only)', () => {
  test('loads URL + API key and saves successfully', async ({ page }) => {
    await page.route('**/api/system/openviking/settings', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          json: {
            serverUrl: 'http://mock-openviking:9090',
            enableMemoryWrite: true,
            enableAutoSync: false,
            apiKeySet: true,
            lastSyncAt: new Date().toISOString(),
          },
        })
      } else if (route.request().method() === 'PUT') {
        const payload = route.request().postDataJSON()
        if (typeof payload.serverUrl === 'string') {
          await route.fulfill({
            status: 200,
            json: {
              serverUrl: payload.serverUrl,
              enableMemoryWrite: payload.enableMemoryWrite ?? true,
              enableAutoSync: payload.enableAutoSync ?? true,
              apiKeySet: true,
            },
          })
        } else {
          await route.fulfill({ status: 400, json: { error: 'Bad Request' } })
        }
      } else {
        await route.fallback()
      }
    })

    await page.route('**/api/system/openviking/status', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({ status: 200, json: { ok: true, latencyMs: 42 } })
      } else {
        await route.fallback()
      }
    })

    await page.goto('/settings')

    const title = page
      .getByRole('heading', { name: 'OpenViking Integration' })
      .or(page.getByRole('heading', { name: 'OpenViking 集成' }))
    await expect(title).toBeVisible()

    await expect(page.getByLabel('Server URL')).toHaveValue('http://mock-openviking:9090')
    await expect(page.getByText(/连通 42ms|Connected 42ms/)).toBeVisible()

    await page.getByLabel('Server URL').fill('http://new-openviking:9090')
    await page.getByRole('button', { name: /^保存$|^Save$/ }).click()

    await expect(
      page
        .getByText('OpenViking 设置已保存')
        .or(page.getByText('OpenViking settings saved'))
        .first(),
    ).toBeVisible()
  })
})

test.describe('OpenViking — project settings (namespace + workspace)', () => {
  test('loads and saves per-project namespace and workspace id', async ({ page }) => {
    await page.route('**/api/projects/prj_mock', async (route) => {
      const method = route.request().method()
      if (method === 'GET') {
        await route.fulfill({ status: 200, json: mockProject })
      } else if (method === 'PATCH') {
        const payload = route.request().postDataJSON()
        if (payload.openvikingNamespace === 'new-namespace') {
          await route.fulfill({
            status: 200,
            json: {
              projectId: mockProject.projectId,
              name: mockProject.name,
              status: mockProject.status,
              updatedAt: new Date().toISOString(),
            },
          })
        } else {
          await route.fulfill({ status: 400, json: { error: 'Bad Request' } })
        }
      } else {
        await route.fallback()
      }
    })

    await page.goto('/projects/prj_mock/settings')

    await expect(page.getByLabel('Namespace')).toHaveValue('mock-namespace')
    await expect(page.getByLabel('Workspace ID')).toHaveValue('mock-workspace')

    await page.getByLabel('Namespace').fill('new-namespace')
    await page.getByRole('button', { name: /保存设置|Save/ }).click()

    await expect(page.getByText('项目设置已保存')).toBeVisible()
  })
})
