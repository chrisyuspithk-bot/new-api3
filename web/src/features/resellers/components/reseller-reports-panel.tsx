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
import { formatQuota } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getGrossProfit, getResellerReconcile } from '../api'

export function ResellerReportsPanel() {
  const { t } = useTranslation()
  const [groupBy, setGroupBy] = useState<'model' | 'customer'>('model')
  const [resellerId, setResellerId] = useState('')
  const [enabled, setEnabled] = useState(false)

  const profitQuery = useQuery({
    queryKey: ['admin-gross-profit', groupBy, resellerId, enabled],
    enabled,
    queryFn: async () => {
      const res = requireServerSuccess(
        await getGrossProfit({
          group_by: groupBy,
          reseller_id: resellerId ? Number(resellerId) : undefined,
        })
      )
      return res.data ?? []
    },
  })

  const reconcileQuery = useQuery({
    queryKey: ['admin-reseller-reconcile', enabled],
    enabled,
    queryFn: async () => {
      const res = requireServerSuccess(await getResellerReconcile({}))
      return res.data!
    },
  })

  return (
    <div className='grid gap-4 lg:grid-cols-2'>
      <Card>
        <CardHeader className='flex flex-row items-center justify-between gap-2'>
          <CardTitle className='text-base'>{t('Gross Profit')}</CardTitle>
          <Button size='sm' onClick={() => setEnabled(true)}>
            {t('Load')}
          </Button>
        </CardHeader>
        <CardContent className='space-y-3'>
          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-1.5'>
              <Label>{t('Group by')}</Label>
              <select
                className='border-input bg-background h-8 w-full rounded-lg border px-2 text-sm'
                value={groupBy}
                onChange={(e) =>
                  setGroupBy(e.target.value as 'model' | 'customer')
                }
              >
                <option value='model'>{t('Model')}</option>
                <option value='customer'>{t('Customer')}</option>
              </select>
            </div>
            <div className='space-y-1.5'>
              <Label>{t('Reseller ID')} ({t('optional')})</Label>
              <Input
                value={resellerId}
                onChange={(e) => setResellerId(e.target.value)}
                placeholder={t('All')}
              />
            </div>
          </div>
          <div className='rounded-md border'>
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
                {(profitQuery.data ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5}>
                      {enabled ? t('No data') : t('Click Load to fetch')}
                    </TableCell>
                  </TableRow>
                ) : (
                  (profitQuery.data ?? []).map((row) => (
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
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{t('Reconcile')}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className='rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Reseller')}</TableHead>
                  <TableHead>{t('Kind')}</TableHead>
                  <TableHead>{t('Detail')}</TableHead>
                  <TableHead>{t('Value')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(reconcileQuery.data?.issues ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4}>
                      {enabled
                        ? t('No issues found')
                        : t('Click Load on Gross Profit to also run reconcile')}
                    </TableCell>
                  </TableRow>
                ) : (
                  (reconcileQuery.data?.issues ?? []).map((issue, idx) => (
                    <TableRow key={`${issue.reseller_id}-${issue.kind}-${idx}`}>
                      <TableCell>
                        {issue.username || issue.reseller_id}
                      </TableCell>
                      <TableCell>{issue.kind}</TableCell>
                      <TableCell className='text-xs'>{issue.detail}</TableCell>
                      <TableCell>{issue.value}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
