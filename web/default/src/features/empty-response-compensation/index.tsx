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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Gift, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TitledCard } from '@/components/ui/titled-card'
import { getSelf } from '@/lib/api'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import {
  claimEmptyResponseCompensations,
  getEmptyResponseCompensations,
} from './api'

const PAGE_SIZE = 20

export function EmptyResponseCompensation() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const setUser = useAuthStore((state) => state.auth.setUser)
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const query = useQuery({
    queryKey: ['empty-response-compensation', 'user', page],
    queryFn: () => getEmptyResponseCompensations(page, PAGE_SIZE),
    refetchInterval: 30_000,
  })
  const claimMutation = useMutation({
    mutationFn: (ids: number[]) => claimEmptyResponseCompensations(ids),
    onSuccess: async (result) => {
      setSelected(new Set())
      await queryClient.invalidateQueries({
        queryKey: ['empty-response-compensation'],
      })
      const self = await getSelf()
      if (self.success && self.data) setUser(self.data as AuthUser)
      if (result.claimed_count > 0) {
        toast.success(
          t('Claimed {{count}} compensation records, credited {{amount}}', {
            count: result.claimed_count,
            amount: formatQuota(result.credited_quota),
          })
        )
      } else {
        toast.info(t('No compensation records were claimed'))
      }
    },
  })

  const data = query.data
  const claimableIds = useMemo(() => {
    if (!data || data.summary.daily_remaining === 0) return []
    return data.records.items
      .filter((record) => record.status === 'pending')
      .map((record) => record.id)
  }, [data])

  useEffect(() => {
    setSelected((current) => {
      const available = new Set(claimableIds)
      return new Set([...current].filter((id) => available.has(id)))
    })
  }, [claimableIds])

  const totalPages = Math.max(
    1,
    Math.ceil((data?.records.total ?? 0) / PAGE_SIZE)
  )
  let qualificationProgress = 0
  if (data) {
    qualificationProgress =
      data.rules.min_qualification_amount <= 0
        ? 100
        : Math.min(
            100,
            (data.summary.qualification_amount /
              data.rules.min_qualification_amount) *
              100
          )
  }

  function toggleAll(checked: boolean) {
    setSelected(checked ? new Set(claimableIds) : new Set())
  }

  function toggleOne(id: number, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Empty Response Compensation')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw className={query.isFetching ? 'animate-spin' : ''} />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4'>
          {!data && (
            <div className='text-muted-foreground py-20 text-center text-sm'>
              {query.isError
                ? t('Failed to load compensation records')
                : t('Loading...')}
            </div>
          )}
          {data && !data.enabled && (
            <Alert>
              <Gift />
              <AlertTitle>{t('Feature disabled')}</AlertTitle>
              <AlertDescription>
                {t('Empty response compensation is not currently enabled')}
              </AlertDescription>
            </Alert>
          )}
          {data?.enabled && (
            <>
              {data.rules.announcement ? (
                <Alert>
                  <Gift />
                  <AlertTitle>{t('Announcement')}</AlertTitle>
                  <AlertDescription className='whitespace-pre-wrap'>
                    {data.rules.announcement}
                  </AlertDescription>
                </Alert>
              ) : null}

              <TitledCard
                title={t('Qualification and balance')}
                description={t(
                  'Qualification includes successful wallet top-ups and redeemed codes'
                )}
                icon={<Gift className='size-4' />}
                disableHoverEffect
              >
                <div className='grid gap-5 lg:grid-cols-[minmax(240px,1fr)_2fr]'>
                  <div className='space-y-2'>
                    <div className='flex items-center justify-between gap-3 text-sm'>
                      <span>{t('Qualification progress')}</span>
                      <Badge
                        variant={data.summary.qualified ? 'default' : 'outline'}
                      >
                        {data.summary.qualified
                          ? t('Qualified')
                          : t('Not qualified')}
                      </Badge>
                    </div>
                    <Progress value={qualificationProgress} />
                    <div className='text-muted-foreground text-xs'>
                      {data.summary.qualification_amount} /{' '}
                      {data.rules.min_qualification_amount}
                    </div>
                  </div>
                  <div className='grid grid-cols-2 border-l-0 lg:grid-cols-4 lg:border-l'>
                    <div className='px-3 py-2'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Pending compensation')}
                      </div>
                      <div className='mt-1 font-semibold'>
                        {formatQuota(data.summary.pending_quota)}
                      </div>
                    </div>
                    <div className='px-3 py-2'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Pending records')}
                      </div>
                      <div className='mt-1 font-semibold'>
                        {data.summary.pending_count}
                      </div>
                    </div>
                    <div className='px-3 py-2'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Claimed today')}
                      </div>
                      <div className='mt-1 font-semibold'>
                        {data.summary.claimed_today}
                      </div>
                    </div>
                    <div className='px-3 py-2'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Remaining today')}
                      </div>
                      <div className='mt-1 font-semibold'>
                        {data.summary.daily_remaining == null
                          ? t('Unlimited')
                          : data.summary.daily_remaining}
                      </div>
                    </div>
                  </div>
                </div>
                <div className='text-muted-foreground mt-4 border-t pt-3 text-xs'>
                  {t(
                    'Records expire after {{days}} days. Client disconnects, upstream errors, subscriptions, and free requests are not compensated.',
                    { days: data.rules.claim_window_days }
                  )}
                </div>
              </TitledCard>

              <TitledCard
                title={t('Compensation records')}
                description={t('The oldest selected records are claimed first')}
                action={
                  <Button
                    className='w-full sm:w-auto'
                    disabled={
                      !data.summary.qualified ||
                      selected.size === 0 ||
                      claimMutation.isPending
                    }
                    onClick={() => claimMutation.mutate([...selected])}
                  >
                    {t('Claim selected')} ({selected.size})
                  </Button>
                }
                disableHoverEffect
              >
                <div className='overflow-x-auto rounded-md border'>
                  <Table className='min-w-[900px]'>
                    <TableHeader>
                      <TableRow>
                        <TableHead className='w-10'>
                          <Checkbox
                            checked={
                              claimableIds.length > 0 &&
                              claimableIds.every((id) => selected.has(id))
                            }
                            onCheckedChange={(checked) =>
                              toggleAll(checked === true)
                            }
                            aria-label={t('Select all pending records')}
                          />
                        </TableHead>
                        <TableHead>{t('Time')}</TableHead>
                        <TableHead>{t('Model')}</TableHead>
                        <TableHead>{t('Tokens')}</TableHead>
                        <TableHead>{t('Charged')}</TableHead>
                        <TableHead>{t('Compensation')}</TableHead>
                        <TableHead>{t('Expires at')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {data.records.items.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={8} className='h-28 text-center'>
                            {t('No compensation records')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        data.records.items.map((record) => (
                          <TableRow key={record.id}>
                            <TableCell>
                              <Checkbox
                                checked={selected.has(record.id)}
                                disabled={!claimableIds.includes(record.id)}
                                onCheckedChange={(checked) =>
                                  toggleOne(record.id, checked === true)
                                }
                                aria-label={t('Select compensation record')}
                              />
                            </TableCell>
                            <TableCell className='text-xs'>
                              {formatTimestamp(record.created_at)}
                            </TableCell>
                            <TableCell className='font-mono text-xs'>
                              {record.model_name}
                            </TableCell>
                            <TableCell>
                              {record.prompt_tokens} /{' '}
                              {record.completion_tokens}
                            </TableCell>
                            <TableCell>
                              {formatQuota(record.original_quota)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(record.compensation_quota)}{' '}
                              <span className='text-muted-foreground text-xs'>
                                ({record.compensation_ratio}%)
                              </span>
                            </TableCell>
                            <TableCell className='text-xs'>
                              {formatTimestamp(record.expires_at)}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={
                                  record.status === 'blocked'
                                    ? 'destructive'
                                    : 'outline'
                                }
                              >
                                {t(record.status)}
                              </Badge>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
                <div className='mt-4 flex items-center justify-between gap-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Page {{page}} of {{total}}', {
                      page,
                      total: totalPages,
                    })}
                  </div>
                  <div className='flex gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page <= 1}
                      onClick={() => setPage((value) => Math.max(1, value - 1))}
                    >
                      {t('Previous')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page >= totalPages}
                      onClick={() =>
                        setPage((value) => Math.min(totalPages, value + 1))
                      }
                    >
                      {t('Next')}
                    </Button>
                  </div>
                </div>
              </TitledCard>
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
