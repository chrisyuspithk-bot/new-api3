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
import {
  downloadInvoiceCsv,
  invoiceCsvFilename,
} from '@/features/invoices/download'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  adminGenerateInvoices,
  adminGetInvoice,
  adminInvoiceCsvUrl,
  adminListInvoices,
} from './api'
import type { Invoice } from './types'

function usd(n: number) {
  return `$${n.toFixed(4)}`
}

export function AdminInvoicesPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [period, setPeriod] = useState('')
  const [scope, setScope] = useState('')
  const [genPeriod, setGenPeriod] = useState(() => {
    const d = new Date()
    d.setMonth(d.getMonth() - 1)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  })
  const [selectedId, setSelectedId] = useState<number | null>(null)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['admin-invoices', period, scope],
    queryFn: async () => {
      const res = requireServerSuccess(
        await adminListInvoices({
          p: 1,
          page_size: 50,
          period: period || undefined,
          scope: scope || undefined,
        })
      )
      return (res.data?.items as Invoice[]) ?? []
    },
  })

  const detail = useQuery({
    queryKey: ['admin-invoice', selectedId],
    enabled: selectedId != null,
    queryFn: async () => {
      const res = requireServerSuccess(await adminGetInvoice(selectedId!))
      return res.data
    },
  })

  const generate = useMutation({
    mutationFn: async () => {
      const res = requireServerSuccess(await adminGenerateInvoices(genPeriod))
      return res.data
    },
    onSuccess: (result) => {
      toast.success(
        t('Generated {{n}} customer + {{m}} reseller invoices', {
          n: result?.customer_invoices ?? 0,
          m: result?.reseller_invoices ?? 0,
        })
      )
      qc.invalidateQueries({ queryKey: ['admin-invoices'] })
      refetch()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const rows = (data ?? []) as Invoice[]

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invoices')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Input
          className='h-8 w-28'
          placeholder='YYYY-MM'
          value={period}
          onChange={(e) => setPeriod(e.target.value)}
        />
        <select
          className='border-input bg-background h-8 rounded-lg border px-2 text-sm'
          value={scope}
          onChange={(e) => setScope(e.target.value)}
        >
          <option value=''>{t('All scopes')}</option>
          <option value='customer'>{t('Customer')}</option>
          <option value='reseller'>{t('Reseller')}</option>
          <option value='admin_rollup'>{t('Admin rollup')}</option>
        </select>
        <Button size='sm' variant='outline' onClick={() => refetch()}>
          {t('Refresh')}
        </Button>
        <Input
          className='h-8 w-28'
          value={genPeriod}
          onChange={(e) => setGenPeriod(e.target.value)}
        />
        <Button
          size='sm'
          disabled={generate.isPending}
          onClick={() => generate.mutate()}
        >
          {t('Generate')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 lg:grid-cols-2'>
          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Period')}</TableHead>
                  <TableHead>{t('Scope')}</TableHead>
                  <TableHead>{t('Party')}</TableHead>
                  <TableHead>{t('Charge')}</TableHead>
                  <TableHead>{t('Cost')}</TableHead>
                  <TableHead>{t('Profit')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading ? (
                  <TableRow>
                    <TableCell colSpan={6}>{t('Loading...')}</TableCell>
                  </TableRow>
                ) : rows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6}>{t('No data')}</TableCell>
                  </TableRow>
                ) : (
                  rows.map((row) => (
                    <TableRow
                      key={row.id}
                      className='cursor-pointer'
                      onClick={() => setSelectedId(row.id)}
                    >
                      <TableCell>{row.period}</TableCell>
                      <TableCell>{row.scope}</TableCell>
                      <TableCell>
                        {row.scope === 'customer'
                          ? row.customer_name
                          : row.scope === 'reseller'
                            ? row.reseller_name
                            : t('All')}
                      </TableCell>
                      <TableCell>{usd(row.charge_usd)}</TableCell>
                      <TableCell>{usd(row.cost_usd)}</TableCell>
                      <TableCell>{usd(row.profit_usd)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <div className='rounded-lg border p-4'>
            {!selectedId ? (
              <p className='text-muted-foreground text-sm'>
                {t('Select an invoice to view details')}
              </p>
            ) : detail.isLoading ? (
              <p>{t('Loading...')}</p>
            ) : detail.data ? (
              <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <h3 className='font-medium'>
                    {detail.data.invoice.period} / {detail.data.invoice.scope}
                  </h3>
                  <a
                    className='text-primary text-sm underline'
                    href={adminInvoiceCsvUrl(detail.data.invoice.id)}
                    onClick={(e) => {
                      e.preventDefault()
                      const inv = detail.data?.invoice
                      if (!inv) return
                      downloadInvoiceCsv(
                        adminInvoiceCsvUrl(inv.id),
                        invoiceCsvFilename(inv.id, inv.period)
                      ).catch(() => toast.error(t('Download failed')))
                    }}
                  >
                    {t('Download CSV')}
                  </a>
                </div>
                <p className='text-sm'>
                  {t('Charge')}: {usd(detail.data.invoice.charge_usd)} (
                  {formatQuota(detail.data.invoice.charge_quota)}) · {t('Cost')}
                  : {usd(detail.data.invoice.cost_usd)} · {t('Profit')}:{' '}
                  {usd(detail.data.invoice.profit_usd)} · {t('Requests')}:{' '}
                  {detail.data.invoice.requests}
                </p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Model')}</TableHead>
                      <TableHead>{t('Requests')}</TableHead>
                      <TableHead>{t('Charge')}</TableHead>
                      <TableHead>{t('Cost')}</TableHead>
                      <TableHead>{t('Profit')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(detail.data.line_items ?? []).map((it) => (
                      <TableRow key={it.model}>
                        <TableCell>{it.model}</TableCell>
                        <TableCell>{it.requests}</TableCell>
                        <TableCell>{usd(it.charge_usd)}</TableCell>
                        <TableCell>{usd(it.cost_usd)}</TableCell>
                        <TableCell>{usd(it.profit_usd)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            ) : (
              <p>{t('No data')}</p>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
