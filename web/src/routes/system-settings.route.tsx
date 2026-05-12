import * as React from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/use-toast'
import { ApiError, describeError } from '@/api/errors'
import {
  useSystemOpenVikingSettingsQuery,
  useSystemOpenVikingStatusQuery,
  useUpdateSystemOpenVikingSettingsMutation,
} from '@/api/system-openviking'
import { formatRelativeTime } from '@/lib/format'

const OPENVIKING_SCHEMA = z.object({
  serverUrl: z.string().url('必须是有效的 URL').min(1, '必填'),
  apiKey: z.string().optional(),
  enableMemoryWrite: z.boolean(),
  enableAutoSync: z.boolean(),
})

type OpenVikingFormValues = z.infer<typeof OPENVIKING_SCHEMA>

export function SystemSettingsRoute() {
  return (
    <div className="h-full overflow-y-auto bg-[hsl(var(--muted)/0.4)]">
      <div className="mx-auto grid max-w-4xl gap-4 p-6">
        <header className="space-y-1">
          <h2 className="text-xl font-semibold text-slate-900">系统设置</h2>
          <p className="text-sm text-slate-500">
            管理系统级集成与默认配置；此处保存的 OpenViking 服务地址与 API Key
            适用于本实例下所有项目。每个项目的 Namespace 与 Workspace ID 请在项目设置中配置。
          </p>
        </header>
        <SystemOpenVikingCard />
      </div>
    </div>
  )
}

function SystemOpenVikingCard() {
  const { t } = useTranslation()
  const { toast } = useToast()

  const settingsQuery = useSystemOpenVikingSettingsQuery()
  const statusQuery = useSystemOpenVikingStatusQuery({ refetchInterval: 30000 })
  const updateMutation = useUpdateSystemOpenVikingSettingsMutation()

  const form = useForm<OpenVikingFormValues>({
    resolver: zodResolver(OPENVIKING_SCHEMA),
    values: settingsQuery.data
      ? {
          serverUrl: settingsQuery.data.serverUrl,
          apiKey: '',
          enableMemoryWrite: settingsQuery.data.enableMemoryWrite,
          enableAutoSync: settingsQuery.data.enableAutoSync,
        }
      : undefined,
  })
  const enableMemoryWrite = useWatch({ control: form.control, name: 'enableMemoryWrite' })
  const enableAutoSync = useWatch({ control: form.control, name: 'enableAutoSync' })

  React.useEffect(() => {
    if (settingsQuery.data) {
      form.reset({
        serverUrl: settingsQuery.data.serverUrl,
        apiKey: '',
        enableMemoryWrite: settingsQuery.data.enableMemoryWrite,
        enableAutoSync: settingsQuery.data.enableAutoSync,
      })
    }
  }, [settingsQuery.data, form])

  const apiKeySet = settingsQuery.data?.apiKeySet

  const handleSave = form.handleSubmit(async (values) => {
    try {
      const payload = {
        serverUrl: values.serverUrl,
        enableMemoryWrite: values.enableMemoryWrite,
        enableAutoSync: values.enableAutoSync,
        apiKey: form.getFieldState('apiKey').isDirty && values.apiKey ? values.apiKey : undefined,
      }
      await updateMutation.mutateAsync(payload)
      toast({ title: t('settings.openviking.saved'), tone: 'success' })
      form.reset(values, { keepDirtyValues: false })
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: t('settings.openviking.saveFailed'), tone: 'destructive' })
      }
    }
  })

  return (
    <Card>
      <CardContent className="space-y-4 p-5">
        <header className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h3 className="text-base font-semibold text-slate-900">
              {t('settings.openviking.title')}
            </h3>
            <div className="mt-1 flex items-center gap-2 text-xs">
              {statusQuery.data?.ok ? (
                <span className="flex items-center gap-1 text-emerald-600">
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  {t('settings.openviking.statusOk', { latency: statusQuery.data.latencyMs ?? 0 })}
                </span>
              ) : (
                <span className="flex items-center gap-1 text-rose-600">
                  <AlertTriangle className="h-3.5 w-3.5" />
                  {t('settings.openviking.statusFail')}
                  {statusQuery.data?.error ? ` - ${statusQuery.data.error}` : ''}
                </span>
              )}
            </div>
          </div>
          <div className="text-right text-xs text-slate-500">
            {settingsQuery.data?.lastSyncAt
              ? `Last Sync: ${formatRelativeTime(settingsQuery.data.lastSyncAt)}`
              : ''}
            {settingsQuery.data?.lastError ? (
              <div className="text-rose-600">{settingsQuery.data.lastError}</div>
            ) : null}
          </div>
        </header>

        {settingsQuery.isLoading ? (
          <div className="flex justify-center p-4">
            <Loader2 className="h-4 w-4 animate-spin text-slate-500" />
          </div>
        ) : (
          <form className="grid gap-4" onSubmit={handleSave}>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="sys-ov-server-url">{t('settings.openviking.serverUrl')}</Label>
                <Input id="sys-ov-server-url" {...form.register('serverUrl')} />
                {form.formState.errors.serverUrl && (
                  <p className="text-[11px] text-rose-600">
                    {form.formState.errors.serverUrl.message}
                  </p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="sys-ov-api-key">{t('settings.openviking.apiKey')}</Label>
                <Input
                  id="sys-ov-api-key"
                  {...form.register('apiKey')}
                  placeholder={
                    apiKeySet
                      ? t('settings.openviking.apiKeyPlaceholderSet')
                      : t('settings.openviking.apiKeyPlaceholderUnset')
                  }
                />
              </div>
            </div>

            <div className="flex gap-6">
              <div className="flex items-center gap-2">
                <Switch
                  checked={enableMemoryWrite}
                  onCheckedChange={(val) =>
                    form.setValue('enableMemoryWrite', val, { shouldDirty: true })
                  }
                />
                <Label>{t('settings.openviking.enableMemoryWrite')}</Label>
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  checked={enableAutoSync}
                  onCheckedChange={(val) =>
                    form.setValue('enableAutoSync', val, { shouldDirty: true })
                  }
                />
                <Label>{t('settings.openviking.enableAutoSync')}</Label>
              </div>
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={!form.formState.isDirty || updateMutation.isPending}>
                {updateMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('common.save')}
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
