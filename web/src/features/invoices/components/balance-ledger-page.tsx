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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'

import type { LedgerPage, LedgerQuery, ResellerLedgerKind } from '../types'

const LEDGER_PAGE_SIZE = 20
const PERIOD_RE = /^\d{4}-\d{2}$/

type BalanceLedgerPageProps = {
  queryKey: string
  loadRows: (query: LedgerQuery) => Promise<LedgerPage>
  kinds: readonly ResellerLedgerKind[]
  showReseller?: boolean
  showCustomer?: boolean
}

function usd(n: number) {
  return `$${n.toFixed(4)}`
}

function currentMonthPeriod(): string {
  // Box / user zone is Asia/Hong_Kong; local calendar month is correct.
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

function ledgerKindLabel(kind: string, t: (key: string) => string): string {
  switch (kind) {
    case 'admin_credit':
      return t('Admin credit')
    case 'admin_debit':
      return t('Admin debit')
    case 'customer_credit':
      return t('Customer credit')
    case 'customer_debit':
      return t('Customer debit')
    default:
      return kind
  }
}

export function BalanceLedgerPage(props: BalanceLedgerPageProps) {
  const { t } = useTranslation()
  const [period, setPeriod] = useState(currentMonthPeriod)
  const [kind, setKind] = useState('')
  const [ledgerPage, setLedgerPage] = useState(1)
  const periodValid = PERIOD_RE.test(period)
  const showReseller = props.showReseller === true
  const showCustomer = props.showCustomer === true
  const columnCount = 6 + (showReseller ? 1 : 0) + (showCustomer ? 1 : 0)

  const ledger = useQuery({
    queryKey: [props.queryKey, period, kind, ledgerPage],
    enabled: periodValid,
    queryFn: () =>
      props.loadRows({
        period,
        kind,
        page: ledgerPage,
        pageSize: LEDGER_PAGE_SIZE,
      }),
  })

  const rows = ledger.data?.items ?? []
  const total = ledger.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / LEDGER_PAGE_SIZE))

  let tableBody: ReactNode
  if (ledger.isLoading) {
    tableBody = (
      <TableRow>
        <TableCell colSpan={columnCount}>{t('Loading...')}</TableCell>
      </TableRow>
    )
  } else if (rows.length === 0) {
    tableBody = (
      <TableRow>
        <TableCell colSpan={columnCount}>{t('No ledger rows')}</TableCell>
      </TableRow>
    )
  } else {
    tableBody = rows.map((row) => (
      <TableRow key={row.id}>
        <TableCell>
          {row.created_at
            ? new Date(row.created_at * 1000).toLocaleString()
            : '—'}
        </TableCell>
        {showReseller ? (
          <TableCell>
            {row.reseller_username || `#${row.reseller_id}`}
          </TableCell>
        ) : null}
        <TableCell>{ledgerKindLabel(row.kind, t)}</TableCell>
        <TableCell>{formatQuota(row.quota_delta)}</TableCell>
        <TableCell>
          {row.money_usd != null ? usd(Number(row.money_usd)) : '—'}
        </TableCell>
        {showCustomer ? (
          <TableCell>
            {row.customer_id
              ? row.customer_username || `#${row.customer_id}`
              : '—'}
          </TableCell>
        ) : null}
        <TableCell>
          {row.operator_id
            ? row.operator_username || `#${row.operator_id}`
            : '—'}
        </TableCell>
        <TableCell>{row.remark || '—'}</TableCell>
      </TableRow>
    ))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invoices')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Input
          className='h-8 w-28'
          placeholder={t('YYYY-MM')}
          aria-label={t('Month')}
          value={period}
          onChange={(e) => {
            setPeriod(e.target.value)
            setLedgerPage(1)
          }}
        />
        <select
          className='border-input bg-background h-8 rounded-lg border px-2 text-sm'
          aria-label={t('Kind')}
          value={kind}
          onChange={(e) => {
            setKind(e.target.value)
            setLedgerPage(1)
          }}
        >
          <option value=''>{t('All kinds')}</option>
          {props.kinds.map((value) => (
            <option key={value} value={value}>
              {ledgerKindLabel(value, t)}
            </option>
          ))}
        </select>
        <Button size='sm' variant='outline' onClick={() => ledger.refetch()}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!periodValid ? (
          <p className='text-muted-foreground text-sm'>
            {t('Enter a period as YYYY-MM to load the balance ledger')}
          </p>
        ) : (
          <div className='space-y-3'>
            <div className='rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    {showReseller ? (
                      <TableHead>{t('Reseller')}</TableHead>
                    ) : null}
                    <TableHead>{t('Kind')}</TableHead>
                    <TableHead>{t('Delta')}</TableHead>
                    <TableHead>{t('USD')}</TableHead>
                    {showCustomer ? (
                      <TableHead>{t('Customer')}</TableHead>
                    ) : null}
                    <TableHead>{t('Operator')}</TableHead>
                    <TableHead>{t('Remark')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>{tableBody}</TableBody>
              </Table>
            </div>
            <div className='text-muted-foreground flex items-center justify-between text-sm'>
              <span>
                {t('Total')}: {total}
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
                  {ledgerPage} / {totalPages}
                </span>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={ledgerPage >= totalPages}
                  onClick={() => setLedgerPage((p) => p + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
