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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
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

import { listResellerLogs } from '../api'

export function ResellerLogsPage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data, isLoading } = useQuery({
    queryKey: ['reseller-logs', page],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listResellerLogs({ p: page, page_size: 20 })
      )
      return res.data!
    },
  })
  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / 20))

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Usage Logs')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Customer')}</TableHead>
                <TableHead>{t('Model')}</TableHead>
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
              ) : items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6}>{t('No logs')}</TableCell>
                </TableRow>
              ) : (
                items.map((row, idx) => (
                  <TableRow key={row.id ?? idx}>
                    <TableCell>
                      {row.created_at
                        ? new Date(row.created_at * 1000).toLocaleString()
                        : '—'}
                    </TableCell>
                    <TableCell>{row.username || '—'}</TableCell>
                    <TableCell>{row.model_name || '—'}</TableCell>
                    <TableCell>{formatQuota(row.quota)}</TableCell>
                    <TableCell>
                      {formatQuota(row.reseller_cost_quota)}
                    </TableCell>
                    <TableCell>
                      {formatQuota(
                        row.profit ?? row.quota - row.reseller_cost_quota
                      )}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        <div className='mt-3 flex justify-end gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground self-center text-sm'>
            {page} / {totalPages}
          </span>
          <Button
            size='sm'
            variant='outline'
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
