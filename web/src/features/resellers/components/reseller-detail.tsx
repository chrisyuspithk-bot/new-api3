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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ledgerKindLabel } from '@/features/invoices/lib/ledger-kind'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getReseller, getResellerLedger, setResellerGroups } from '../api'
import { ResellerCostTab } from './reseller-cost-tab'
import { ResellerCustomersTab } from './reseller-customers-tab'

const LEDGER_PAGE_SIZE = 20

export function ResellerDetail({ id }: { id: number }) {
  const { t, i18n } = useTranslation()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const qc = useQueryClient()
  const [groupsText, setGroupsText] = useState<string | null>(null)
  const [ledgerPage, setLedgerPage] = useState(1)

  const detail = useQuery({
    queryKey: ['admin-reseller', id],
    queryFn: async () => {
      const res = requireServerSuccess(await getReseller(id))
      return res.data ?? null
    },
  })

  const ledger = useQuery({
    queryKey: ['admin-reseller-ledger', id, ledgerPage],
    queryFn: async () => {
      const res = requireServerSuccess(
        await getResellerLedger(id, {
          p: ledgerPage,
          page_size: LEDGER_PAGE_SIZE,
        })
      )
      return res.data ?? null
    },
  })

  const reseller = detail.data

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
        <TableCell colSpan={7}>{t('Loading...')}</TableCell>
      </TableRow>
    )
  } else if (ledger.isError) {
    ledgerBody = (
      <TableRow>
        <TableCell colSpan={7}>{t('Failed to load')}</TableCell>
      </TableRow>
    )
  } else if (ledgerRows.length === 0) {
    ledgerBody = (
      <TableRow>
        <TableCell colSpan={7}>{t('No ledger rows')}</TableCell>
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
          {row.money_usd != null ? `$${Number(row.money_usd).toFixed(4)}` : '—'}
        </TableCell>
        <TableCell>
          {row.customer_id
            ? row.customer_username || `#${row.customer_id}`
            : '—'}
        </TableCell>
        <TableCell>
          {row.operator_id
            ? row.operator_username || `#${row.operator_id}`
            : '—'}
        </TableCell>
        <TableCell>{row.remark || '—'}</TableCell>
      </TableRow>
    ))
  }
  const groupValue = groupsText ?? (reseller?.groups ?? []).join(', ')

  const saveGroups = async () => {
    try {
      const groups = groupValue
        .split(',')
        .map((g) => g.trim())
        .filter(Boolean)
      requireServerSuccess(await setResellerGroups(id, groups))
      toast.success(t('Groups updated'))
      qc.invalidateQueries({ queryKey: ['admin-reseller', id] })
    } catch (error) {
      handleServerError(error, t('Failed to update groups'))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {reseller ? `${t('Reseller')}: ${reseller.username}` : t('Reseller')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' size='sm' render={<Link to='/resellers' />}>
          {t('Back')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!reseller ? (
          <div>{t('Loading...')}</div>
        ) : (
          <Tabs defaultValue='overview'>
            <TabsList>
              <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
              <TabsTrigger value='customers'>{t('Customers')}</TabsTrigger>
              <TabsTrigger value='groups'>{t('Groups')}</TabsTrigger>
              <TabsTrigger value='prices'>
                {t('Reseller cost (costFactor)')}
              </TabsTrigger>
              <TabsTrigger value='ledger'>{t('Ledger')}</TabsTrigger>
            </TabsList>

            <TabsContent value='overview' className='mt-4'>
              <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-5'>
                {[
                  [t('Balance'), formatQuota(reseller.quota)],
                  [t('Used'), formatQuota(reseller.used_quota)],
                  [
                    t('Default markup (Reseller→Customer)'),
                    String(reseller.profile?.default_markup ?? '—'),
                  ],
                  [
                    t('Default costFactor (Admin→Reseller)'),
                    String(reseller.profile?.default_cost_factor ?? '—'),
                  ],
                  [t('Customers'), String(reseller.customer_count ?? 0)],
                ].map(([label, value]) => (
                  <Card key={label}>
                    <CardHeader className='pb-2'>
                      <CardTitle className='text-muted-foreground text-sm font-medium'>
                        {label}
                      </CardTitle>
                    </CardHeader>
                    <CardContent className='text-2xl font-semibold'>
                      {value}
                    </CardContent>
                  </Card>
                ))}
              </div>
              <Card className='mt-4'>
                <CardContent className='space-y-1 pt-4 text-sm'>
                  <div>
                    {t('Brand')}: {reseller.profile?.display_name || '—'}
                  </div>
                  <div>
                    {t('Default Group')}: {reseller.group}
                  </div>
                  <div>
                    {t('Max Customers')}:{' '}
                    {reseller.profile?.max_customers || t('Unlimited')}
                  </div>
                  <div>
                    {t('Allow Redemption')}:{' '}
                    {reseller.profile?.allow_redemption ? t('Yes') : t('No')}
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value='customers' className='mt-4'>
              <ResellerCustomersTab resellerId={id} />
            </TabsContent>

            <TabsContent value='groups' className='mt-4 space-y-3'>
              <Label>{t('Allowed Groups')}</Label>
              <Input
                value={groupValue}
                onChange={(e) => setGroupsText(e.target.value)}
              />
              <Button size='sm' onClick={saveGroups}>
                {t('Save Groups')}
              </Button>
            </TabsContent>

            <TabsContent value='prices' className='mt-4'>
              <ResellerCostTab resellerId={id} />
            </TabsContent>

            <TabsContent value='ledger' className='mt-4 space-y-3'>
              <div className='rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('Kind')}</TableHead>
                      <TableHead>{t('Delta')}</TableHead>
                      <TableHead>{t('USD')}</TableHead>
                      <TableHead>{t('Customer')}</TableHead>
                      <TableHead>{t('Operator')}</TableHead>
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
            </TabsContent>
          </Tabs>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
