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
import { toast } from 'sonner'
import {
  downloadInvoiceCsv,
  invoiceCsvFilename,
} from '@/features/invoices/download'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  customerInvoiceCsvUrl,
  customerListInvoices,
} from '@/features/invoices/api'
import type { Invoice } from '@/features/invoices/types'
import { requireServerSuccess } from '@/lib/server-error-message'

function usd(n: number) {
  return `$${n.toFixed(4)}`
}

export function InvoiceHistoryCard() {
  const { t } = useTranslation()
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['customer-invoices'],
    queryFn: async () => {
      const res = requireServerSuccess(
        await customerListInvoices({ p: 1, page_size: 20 })
      )
      return (res.data?.items as Invoice[]) ?? []
    },
  })

  const rows = data ?? []

  return (
    <div className='rounded-lg border p-4'>
      <div className='mb-3 flex items-center justify-between'>
        <h3 className='font-medium'>{t('Invoice History')}</h3>
        <Button size='sm' variant='outline' onClick={() => refetch()}>
          {t('Refresh')}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Period')}</TableHead>
            <TableHead>{t('Charge')}</TableHead>
            <TableHead>{t('Requests')}</TableHead>
            <TableHead>{t('CSV')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            <TableRow>
              <TableCell colSpan={4}>{t('Loading...')}</TableCell>
            </TableRow>
          ) : rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={4}>{t('No invoices yet')}</TableCell>
            </TableRow>
          ) : (
            rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell>{row.period}</TableCell>
                <TableCell>{usd(row.charge_usd)}</TableCell>
                <TableCell>{row.requests}</TableCell>
                <TableCell>
                  <a
                    className='text-primary text-sm underline'
                    href={customerInvoiceCsvUrl(row.id)}
                    onClick={(e) => {
                      e.preventDefault()
                      downloadInvoiceCsv(
                        customerInvoiceCsvUrl(row.id),
                        invoiceCsvFilename(row.id, row.period)
                      ).catch(() => toast.error(t('Download failed')))
                    }}
                  >
                    {t('Download')}
                  </a>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
