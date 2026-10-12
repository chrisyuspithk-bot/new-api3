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

import { getGrossProfit } from '../api'

export function ResellerReportsPage() {
  const { t } = useTranslation()
  const [groupBy, setGroupBy] = useState<'model' | 'customer'>('model')
  const [enabled, setEnabled] = useState(true)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['reseller-gross-profit', groupBy],
    enabled,
    queryFn: async () => {
      const res = requireServerSuccess(
        await getGrossProfit({ group_by: groupBy })
      )
      return res.data ?? []
    },
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Gross Profit')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <select
          className='border-input bg-background h-8 rounded-lg border px-2 text-sm'
          value={groupBy}
          onChange={(e) => setGroupBy(e.target.value as 'model' | 'customer')}
        >
          <option value='model'>{t('By Model')}</option>
          <option value='customer'>{t('By Customer')}</option>
        </select>
        <Button
          size='sm'
          variant='outline'
          onClick={() => {
            setEnabled(true)
            refetch()
          }}
        >
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Key')}</TableHead>
                <TableHead>{t('Requests')}</TableHead>
                <TableHead>{t('Charge')}</TableHead>
                <TableHead>{t('Cost')}</TableHead>
                <TableHead>{t('Profit')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5}>{t('Loading...')}</TableCell>
                </TableRow>
              ) : (data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5}>{t('No data')}</TableCell>
                </TableRow>
              ) : (
                (data ?? []).map((row) => (
                  <TableRow key={row.key}>
                    <TableCell>{row.key}</TableCell>
                    <TableCell>{row.requests}</TableCell>
                    <TableCell>{formatQuota(row.charge_quota)}</TableCell>
                    <TableCell>{formatQuota(row.cost_quota)}</TableCell>
                    <TableCell>{formatQuota(row.profit_quota)}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
