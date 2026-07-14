/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useRef } from 'react'
import { useFieldArray, useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { getAdminEmptyResponseCompensations } from '@/features/empty-response-compensation/api'
import { formatQuota, formatTimestamp } from '@/lib/format'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const modelRatioSchema = z.object({
  model: z.string().trim().min(1),
  ratio: z.coerce.number().int().min(1).max(100),
})

const schema = z.object({
  enabled: z.boolean(),
  modelRatios: z.array(modelRatioSchema),
  minQualificationAmount: z.coerce.number().int().min(0),
  inputTokenThreshold: z.coerce.number().int().min(0),
  outputTokenThreshold: z.coerce.number().int().min(0),
  claimWindowDays: z.coerce.number().int().min(1),
  dailyClaimLimit: z.coerce.number().int().min(0),
  overclockWindowMinutes: z.coerce.number().int().min(0),
  overclockEmptyCount: z.coerce.number().int().min(0),
  announcement: z.string(),
})

type Values = z.infer<typeof schema>

type EmptyResponseCompensationDefaults = {
  enabled: boolean
  modelRatios: string
  minQualificationAmount: number
  inputTokenThreshold: number
  outputTokenThreshold: number
  claimWindowDays: number
  dailyClaimLimit: number
  overclockWindowMinutes: number
  overclockEmptyCount: number
  announcement: string
}

function parseModelRatios(raw: string): Values['modelRatios'] {
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.entries(parsed)
      .filter(
        ([model, ratio]) =>
          model.trim() !== '' &&
          typeof ratio === 'number' &&
          Number.isInteger(ratio) &&
          ratio >= 1 &&
          ratio <= 100
      )
      .map(([model, ratio]) => ({ model, ratio: ratio as number }))
      .sort((a, b) => a.model.localeCompare(b.model))
  } catch {
    return []
  }
}

function serializeModelRatios(rows: Values['modelRatios']) {
  const ratios: Record<string, number> = {}
  for (const row of [...rows].sort((a, b) => a.model.localeCompare(b.model))) {
    ratios[row.model.trim()] = row.ratio
  }
  return JSON.stringify(ratios)
}

export function EmptyResponseCompensationSection({
  defaultValues,
}: {
  defaultValues: EmptyResponseCompensationDefaults
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const parsedDefaults: Values = {
    ...defaultValues,
    modelRatios: parseModelRatios(defaultValues.modelRatios),
  }
  const savedValuesRef = useRef(parsedDefaults)
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: parsedDefaults,
  })
  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'modelRatios',
  })
  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')
  const adminQuery = useQuery({
    queryKey: ['empty-response-compensation', 'admin'],
    queryFn: () => getAdminEmptyResponseCompensations(1, 20),
    refetchInterval: 30_000,
  })

  async function onSubmit(values: Values) {
    const modelNames = values.modelRatios.map((item) => item.model.trim())
    if (new Set(modelNames).size !== modelNames.length) {
      toast.error(t('Each model can only appear once'))
      return
    }

    const currentRatios = serializeModelRatios(values.modelRatios)
    const savedValues = savedValuesRef.current
    const originalRatios = serializeModelRatios(savedValues.modelRatios)
    const optionValues: Array<{
      key: string
      value: string
      original: string
    }> = [
      {
        key: 'empty_response_compensation_setting.enabled',
        value: String(values.enabled),
        original: String(savedValues.enabled),
      },
      {
        key: 'empty_response_compensation_setting.model_ratios',
        value: currentRatios,
        original: originalRatios,
      },
      {
        key: 'empty_response_compensation_setting.min_qualification_amount',
        value: String(values.minQualificationAmount),
        original: String(savedValues.minQualificationAmount),
      },
      {
        key: 'empty_response_compensation_setting.input_token_threshold',
        value: String(values.inputTokenThreshold),
        original: String(savedValues.inputTokenThreshold),
      },
      {
        key: 'empty_response_compensation_setting.output_token_threshold',
        value: String(values.outputTokenThreshold),
        original: String(savedValues.outputTokenThreshold),
      },
      {
        key: 'empty_response_compensation_setting.claim_window_days',
        value: String(values.claimWindowDays),
        original: String(savedValues.claimWindowDays),
      },
      {
        key: 'empty_response_compensation_setting.daily_claim_limit',
        value: String(values.dailyClaimLimit),
        original: String(savedValues.dailyClaimLimit),
      },
      {
        key: 'empty_response_compensation_setting.overclock_window_minutes',
        value: String(values.overclockWindowMinutes),
        original: String(savedValues.overclockWindowMinutes),
      },
      {
        key: 'empty_response_compensation_setting.overclock_empty_count',
        value: String(values.overclockEmptyCount),
        original: String(savedValues.overclockEmptyCount),
      },
      {
        key: 'empty_response_compensation_setting.announcement',
        value: values.announcement,
        original: savedValues.announcement,
      },
    ]

    const updates = optionValues.filter((item) => item.value !== item.original)
    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }
    for (const update of updates) {
      await updateOption.mutateAsync({ key: update.key, value: update.value })
    }
    savedValuesRef.current = {
      ...values,
      modelRatios: values.modelRatios.map((item) => ({ ...item })),
    }
    form.reset(values)
  }

  const adminData = adminQuery.data

  return (
    <SettingsSection title={t('Empty Response Compensation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save compensation settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Enable empty response compensation')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Only new eligible wallet requests are recorded after this switch is enabled'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='space-y-3' data-settings-form-span='full'>
            <div className='flex flex-wrap items-end justify-between gap-3'>
              <div>
                <FormLabel>{t('Compensated models')}</FormLabel>
                <FormDescription>
                  {t(
                    'Use the exact model name from the client request and set a compensation ratio from 1% to 100%'
                  )}
                </FormDescription>
              </div>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => append({ model: '', ratio: 100 })}
              >
                <Plus />
                {t('Add model')}
              </Button>
            </div>
            {fields.length === 0 ? (
              <div className='text-muted-foreground rounded-md border border-dashed px-3 py-8 text-center text-sm'>
                {t('No models are configured for compensation')}
              </div>
            ) : (
              <div className='space-y-2'>
                {fields.map((field, index) => (
                  <div
                    key={field.id}
                    className='grid min-w-0 grid-cols-[minmax(0,1fr)_110px_36px] items-start gap-2'
                  >
                    <FormField
                      control={form.control}
                      name={`modelRatios.${index}.model`}
                      render={({ field: modelField }) => (
                        <FormItem>
                          <FormControl>
                            <Input
                              placeholder={t('Exact model name')}
                              {...modelField}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name={`modelRatios.${index}.ratio`}
                      render={({ field: ratioField }) => (
                        <FormItem>
                          <FormControl>
                            <div className='relative'>
                              <Input
                                type='number'
                                min={1}
                                max={100}
                                className='pr-7'
                                {...ratioField}
                              />
                              <span className='text-muted-foreground pointer-events-none absolute top-1/2 right-2 -translate-y-1/2 text-sm'>
                                %
                              </span>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      title={t('Remove model')}
                      aria-label={t('Remove model')}
                      onClick={() => remove(index)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div
            className='grid gap-5 sm:grid-cols-2'
            data-settings-form-span='full'
          >
            <FormField
              control={form.control}
              name='minQualificationAmount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Minimum qualification amount')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={0} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Successful wallet top-ups and redeemed codes count toward qualification'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='claimWindowDays'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Claim window in days')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={1} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The ratio and amount are locked when the empty response occurs'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='inputTokenThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Minimum input tokens')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={0} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='outputTokenThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum empty output tokens')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={0} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='dailyClaimLimit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Daily claim limit')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={0} {...field} />
                  </FormControl>
                  <FormDescription>{t('0 means unlimited')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className='grid grid-cols-2 gap-2'>
              <FormField
                control={form.control}
                name='overclockWindowMinutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Risk window in minutes')}</FormLabel>
                    <FormControl>
                      <Input type='number' min={0} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='overclockEmptyCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Allowed empty responses')}</FormLabel>
                    <FormControl>
                      <Input type='number' min={0} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          <FormField
            control={form.control}
            name='announcement'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Compensation announcement')}</FormLabel>
                <FormControl>
                  <Textarea rows={3} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      <div className='border-t pt-5'>
        <div className='mb-3 flex items-center justify-between gap-3'>
          <h3 className='text-sm font-semibold'>{t('Compensation ledger')}</h3>
          <span className='text-muted-foreground text-xs'>
            {enabled ? t('Refreshing every 30 seconds') : t('Feature disabled')}
          </span>
        </div>
        {adminData ? (
          <>
            <div className='mb-4 grid grid-cols-2 border-y sm:grid-cols-4'>
              {[
                [t('Pending'), adminData.summary.pending_count],
                [t('Claimed'), adminData.summary.claimed_count],
                [t('Blocked'), adminData.summary.blocked_count],
                [t('Expired'), adminData.summary.expired_count],
              ].map(([label, value]) => (
                <div key={String(label)} className='px-3 py-3'>
                  <div className='text-muted-foreground text-xs'>{label}</div>
                  <div className='mt-1 text-lg font-semibold'>{value}</div>
                </div>
              ))}
            </div>
            <div className='overflow-x-auto rounded-md border'>
              <Table className='min-w-[760px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Tokens')}</TableHead>
                    <TableHead>{t('Compensation')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {adminData.records.items.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className='h-24 text-center'>
                        {t('No compensation records')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    adminData.records.items.map((record) => (
                      <TableRow key={record.id}>
                        <TableCell className='text-xs'>
                          {formatTimestamp(record.created_at)}
                        </TableCell>
                        <TableCell>
                          {record.username || `#${record.user_id}`}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {record.model_name}
                        </TableCell>
                        <TableCell>
                          {record.prompt_tokens} / {record.completion_tokens}
                        </TableCell>
                        <TableCell>
                          {formatQuota(record.compensation_quota)} (
                          {record.compensation_ratio}%)
                        </TableCell>
                        <TableCell>
                          <Badge variant='outline'>{t(record.status)}</Badge>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </>
        ) : (
          <div className='text-muted-foreground rounded-md border px-3 py-10 text-center text-sm'>
            {adminQuery.isError
              ? t('Failed to load compensation records')
              : t('Loading...')}
          </div>
        )}
      </div>
    </SettingsSection>
  )
}
