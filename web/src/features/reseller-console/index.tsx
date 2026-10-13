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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ledgerKindLabel } from '@/features/invoices/lib/ledger-kind'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getLedger, getResellerSelf, updateResellerMarkup } from './api'

const LEDGER_PAGE_SIZE = 20

export function ResellerOverview() {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [markup, setMarkup] = useState<string | null>(null)
  const [ledgerPage, setLedgerPage] = useState(1)

  const self = useQuery({
    queryKey: ['reseller-self'],
    queryFn: async () => {
      const res = requireServerSuccess(await getResellerSelf())
      return res.data ?? null
    },
  })

  const ledger = useQuery({
    queryKey: ['reseller-ledger', ledgerPage],
    queryFn: async () => {
      const res = requireServerSuccess(
        await getLedger({ p: ledgerPage, page_size: LEDGER_PAGE_SIZE })
      )
      return res.data ?? null
    },
  })

  const data = self.data
  const markupValue = markup ?? String(data?.profile?.default_markup ?? '1')

  const saveMarkup = async () => {
    try {
      requireServerSuccess(await updateResellerMarkup(Number(markupValue)))
      toast.success(t('Default markup updated'))
      qc.invalidateQueries({ queryKey: ['reseller-self'] })
    } catch (error) {
      handleServerError(error, t('Failed to update markup'))
    }
  }

  const ledgerRows = ledger.data?.items ?? []
  const ledgerTotal = ledger.data?.total ?? 0
  const ledgerTotalPages = Math.max(
    1,
    Math.ceil(ledgerTotal / LEDGER_PAGE_SIZE)
  )
  let ledgerBody: ReactNode
  if (ledger.isLoading) {
    ledgerBody = (
      <TableRow>
        <TableCell colSpan={5}>{t('Loading...')}</TableCell>
      </TableRow>
    )
  } else if (ledger.isError) {
    ledgerBody = (
      <TableRow>
        <TableCell colSpan={5}>{t('Failed to load')}</TableCell>
      </TableRow>
    )
  } else if (ledgerRows.length === 0) {
    ledgerBody = (
      <TableRow>
        <TableCell colSpan={5}>{t('No ledger rows')}</TableCell>
      </TableRow>
    )
  } else {
    ledgerBody = ledgerRows.map((row) => (
      <TableRow key={row.id}>
        <TableCell>
          {row.created_at
            ? new Date(row.created_at * 1000).toLocaleString(intlLocale)
            : '—'}
        </TableCell>
        <TableCell>{ledgerKindLabel(row.kind, t)}</TableCell>
        <TableCell>{formatQuota(row.quota_delta)}</TableCell>
        <TableCell>
          {row.customer_id
            ? row.customer_username || `#${row.customer_id}`
            : '—'}
        </TableCell>
        <TableCell>{row.remark || '—'}</TableCell>
      </TableRow>
    ))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Reseller Overview')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button render={<Link to='/wallet' />}>
          {t('Top up cost balance')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!data ? (
          <div>{t('Loading...')}</div>
        ) : (
          <div className='space-y-6'>
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              {[
                [t('Cost Balance'), formatQuota(data.quota)],
                [t('Used Cost'), formatQuota(data.used_quota)],
                [t('Customers'), String(data.customer_count ?? 0)],
                [
                  t('Brand'),
                  data.profile?.display_name || data.display_name || '—',
                ],
              ].map(([label, value]) => (
                <Card key={label}>
                  <CardHeader className='pb-2'>
                    <CardTitle className='text-muted-foreground text-sm'>
                      {label}
                    </CardTitle>
                  </CardHeader>
                  <CardContent className='text-2xl font-semibold'>
                    {value}
                  </CardContent>
                </Card>
              ))}
            </div>

            <Card>
              <CardHeader>
                <CardTitle className='text-base'>
                  {t('Default Markup')}
                </CardTitle>
              </CardHeader>
              <CardContent className='flex flex-wrap items-end gap-3'>
                <div className='space-y-1.5'>
                  <Label>{t('Markup')}</Label>
                  <Input
                    type='number'
                    step='0.0001'
                    min='0.0001'
                    value={markupValue}
                    onChange={(e) => setMarkup(e.target.value)}
                  />
                </div>
                <Button size='sm' onClick={saveMarkup}>
                  {t('Save')}
                </Button>
                <p className='text-muted-foreground w-full text-xs'>
                  {t('Allowed groups')}: {(data.groups ?? []).join(', ') || '—'}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='text-base'>
                  {t('Recent Ledger')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className='space-y-3'>
                  <div className='rounded-lg border'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Time')}</TableHead>
                          <TableHead>{t('Kind')}</TableHead>
                          <TableHead>{t('Delta')}</TableHead>
                          <TableHead>{t('Customer')}</TableHead>
                          <TableHead>{t('Remark')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>{ledgerBody}</TableBody>
                    </Table>
                  </div>
                  <div className='text-muted-foreground flex items-center justify-between text-sm'>
                    <span>
                      {t('Total')}: {ledgerTotal}
                    </span>
                    <div className='flex items-center gap-2'>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={ledgerPage <= 1}
                        onClick={() => setLedgerPage((p) => p - 1)}
                      >
                        {t('Previous')}
                      </Button>
                      <span>
                        {ledgerPage} / {ledgerTotalPages}
                      </span>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={ledgerPage >= ledgerTotalPages}
                        onClick={() => setLedgerPage((p) => p + 1)}
                      >
                        {t('Next')}
                      </Button>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
