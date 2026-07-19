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
import { useQuery } from '@tanstack/react-query'
import { Activity, Clock3, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { getPerfMetricsMonitor } from '@/features/performance-metrics/api'
import {
  getSuccessRateDotClass,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type {
  MonitorMinutePoint,
  MonitorModel,
} from '@/features/performance-metrics/types'
import { formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

const WINDOW_MINUTES = 15

function ModelStatusCard({ model }: { model: MonitorModel }) {
  const { t } = useTranslation()
  const healthy = model.success_rate >= 90
  const firstObservedRate = model.timeline.find(
    (point) => point.success_rate != null
  )?.success_rate
  let latestRate = firstObservedRate ?? model.success_rate
  const displayTimeline = model.timeline.map((point) => {
    if (point.success_rate != null) {
      latestRate = point.success_rate
      return point
    }
    return { ...point, success_rate: latestRate }
  })

  return (
    <Card className='gap-0 py-0' data-card-hover='false'>
      <CardContent className='p-3 sm:p-4'>
        <div className='flex min-w-0 items-center justify-between gap-3'>
          <div className='min-w-0 font-mono text-sm font-semibold'>
            <span className='block truncate' title={model.model_name}>
              {model.model_name}
            </span>
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <Badge
              variant='outline'
              className={cn(
                'gap-1 border-current/20 px-2 py-0.5',
                healthy
                  ? 'text-emerald-600 dark:text-emerald-400'
                  : 'text-amber-600 dark:text-amber-400'
              )}
            >
              <span
                className={cn(
                  'size-1.5 rounded-full',
                  healthy ? 'bg-emerald-500' : 'bg-amber-500'
                )}
              />
              {healthy ? t('Healthy') : t('Degraded')}
            </Badge>
            <span
              className={cn(
                'text-sm font-bold tabular-nums',
                getSuccessRateTextClass(model.success_rate)
              )}
            >
              {model.success_rate.toFixed(0)}%
            </span>
          </div>
        </div>

        <div
          className='mt-5 grid h-4 grid-cols-[repeat(15,minmax(0,1fr))] gap-1'
          aria-label={t('Success rate')}
        >
          {displayTimeline.map((point) => (
            <MinuteBar key={point.ts} point={point} />
          ))}
        </div>
        <div className='text-muted-foreground mt-1.5 flex items-center justify-between text-[11px]'>
          <span>{t('15 minutes ago')}</span>
          <span>{t('Now')}</span>
        </div>
      </CardContent>
    </Card>
  )
}

function MinuteBar({ point }: { point: MonitorMinutePoint }) {
  const { t } = useTranslation()
  const time = new Date(point.ts * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
  const title =
    point.success_rate == null
      ? t('Minute {{time}}: no recent status', { time })
      : t('Minute {{time}}: {{rate}} success rate', {
          time,
          rate: `${point.success_rate.toFixed(0)}%`,
        })

  return (
    <span
      title={title}
      className={cn(
        'h-4 min-w-0 rounded-[2px]',
        point.success_rate == null
          ? 'bg-muted'
          : getSuccessRateDotClass(point.success_rate)
      )}
    />
  )
}

export function ModelMonitor() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['perf-metrics-monitor', WINDOW_MINUTES],
    queryFn: () => getPerfMetricsMonitor(WINDOW_MINUTES),
    refetchInterval: 30_000,
    staleTime: 15_000,
    retry: false,
  })
  const data = query.data?.data

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Model Monitor')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='icon-sm'
          onClick={() => query.refetch()}
          disabled={query.isFetching}
          title={t('Refresh')}
          aria-label={t('Refresh')}
        >
          <RefreshCw className={query.isFetching ? 'animate-spin' : ''} />
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-[1600px] flex-col gap-3 sm:gap-4'>
          <div className='rounded-md border p-3 sm:p-4'>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  <Activity className='text-primary size-4' />
                  <h3 className='text-sm font-semibold sm:text-base'>
                    {t('15-minute model success rate')}
                  </h3>
                </div>
                <p className='text-muted-foreground mt-1 text-xs sm:text-sm'>
                  {t(
                    'A request counts as successful when it is not recorded as an error.'
                  )}
                </p>
              </div>
              <Badge variant='outline' className='w-fit gap-1.5'>
                <Clock3 className='size-3' />
                {t('15-minute window')}
              </Badge>
            </div>

            {data && (
              <div className='text-muted-foreground mt-4 grid gap-3 border-t pt-3 text-xs sm:grid-cols-3'>
                <div>
                  <div>{t('Last refreshed')}</div>
                  <div className='text-foreground mt-0.5 font-medium tabular-nums'>
                    {formatTimestamp(data.refreshed_at)}
                  </div>
                </div>
                <div>
                  <div>{t('Window start')}</div>
                  <div className='text-foreground mt-0.5 font-medium tabular-nums'>
                    {formatTimestamp(data.window_start)}
                  </div>
                </div>
                <div>
                  <div>{t('Window end')}</div>
                  <div className='text-foreground mt-0.5 font-medium tabular-nums'>
                    {formatTimestamp(data.window_end)}
                  </div>
                </div>
              </div>
            )}
          </div>

          {!data && (
            <div className='text-muted-foreground py-20 text-center text-sm'>
              {query.isError
                ? t('Failed to load model monitor data')
                : t('Loading...')}
            </div>
          )}

          {data && data.models.length === 0 && (
            <div className='text-muted-foreground rounded-md border py-20 text-center text-sm'>
              {t('No model activity was recorded in the last 15 minutes.')}
            </div>
          )}

          {data && data.models.length > 0 && (
            <div className='grid grid-cols-1 gap-3 lg:grid-cols-2 2xl:grid-cols-3'>
              {data.models.map((model) => (
                <ModelStatusCard key={model.model_name} model={model} />
              ))}
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
