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
import { useState } from 'react'
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
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  deleteResellerModelPrice,
  getReseller,
  getResellerLedger,
  listResellerModelPrices,
  setResellerGroups,
  upsertResellerModelPrice,
} from '../api'

export function ResellerDetail({ id }: { id: number }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [groupsText, setGroupsText] = useState<string | null>(null)
  const [modelName, setModelName] = useState('')
  const [costMultiplier, setCostMultiplier] = useState('1')

  const detail = useQuery({
    queryKey: ['admin-reseller', id],
    queryFn: async () => {
      const res = requireServerSuccess(await getReseller(id))
      return res.data!
    },
  })

  const prices = useQuery({
    queryKey: ['admin-reseller-prices', id],
    queryFn: async () => {
      const res = requireServerSuccess(await listResellerModelPrices(id))
      return res.data ?? []
    },
  })

  const ledger = useQuery({
    queryKey: ['admin-reseller-ledger', id],
    queryFn: async () => {
      const res = requireServerSuccess(
        await getResellerLedger(id, { p: 1, page_size: 50 })
      )
      return res.data!
    },
  })

  const reseller = detail.data
  const groupValue =
    groupsText ?? (reseller?.groups ?? []).join(', ')

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

  const addPrice = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      requireServerSuccess(
        await upsertResellerModelPrice(id, {
          model_name: modelName,
          cost_multiplier: Number(costMultiplier),
        })
      )
      toast.success(t('Model price saved'))
      setModelName('')
      qc.invalidateQueries({ queryKey: ['admin-reseller-prices', id] })
    } catch (error) {
      handleServerError(error, t('Failed to save model price'))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {reseller
          ? `${t('Reseller')}: ${reseller.username}`
          : t('Reseller')}
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
              <TabsTrigger value='groups'>{t('Groups')}</TabsTrigger>
              <TabsTrigger value='prices'>{t('Model Prices')}</TabsTrigger>
              <TabsTrigger value='ledger'>{t('Ledger')}</TabsTrigger>
            </TabsList>

            <TabsContent value='overview' className='mt-4'>
              <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
                {[
                  [t('Balance'), formatQuota(reseller.quota)],
                  [t('Used'), formatQuota(reseller.used_quota)],
                  [
                    t('Markup'),
                    String(reseller.profile?.default_markup ?? '—'),
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

            <TabsContent value='prices' className='mt-4 space-y-4'>
              <form
                onSubmit={addPrice}
                className='flex flex-wrap items-end gap-3'
              >
                <div className='space-y-1.5'>
                  <Label>{t('Model')}</Label>
                  <Input
                    value={modelName}
                    onChange={(e) => setModelName(e.target.value)}
                    required
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Cost Multiplier')}</Label>
                  <Input
                    type='number'
                    step='0.0001'
                    min='0.0001'
                    value={costMultiplier}
                    onChange={(e) => setCostMultiplier(e.target.value)}
                    required
                  />
                </div>
                <Button type='submit' size='sm'>
                  {t('Save')}
                </Button>
              </form>
              <div className='rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Model')}</TableHead>
                      <TableHead>{t('Cost Multiplier')}</TableHead>
                      <TableHead>{t('Ratio')}</TableHead>
                      <TableHead>{t('Price')}</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(prices.data ?? []).length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={5}>{t('No overrides')}</TableCell>
                      </TableRow>
                    ) : (
                      (prices.data ?? []).map((row) => (
                        <TableRow key={row.model_name}>
                          <TableCell>{row.model_name}</TableCell>
                          <TableCell>{row.cost_multiplier ?? '—'}</TableCell>
                          <TableCell>{row.model_ratio ?? '—'}</TableCell>
                          <TableCell>{row.model_price ?? '—'}</TableCell>
                          <TableCell>
                            <Button
                              size='sm'
                              variant='ghost'
                              onClick={async () => {
                                try {
                                  requireServerSuccess(
                                    await deleteResellerModelPrice(
                                      id,
                                      row.model_name
                                    )
                                  )
                                  qc.invalidateQueries({
                                    queryKey: ['admin-reseller-prices', id],
                                  })
                                } catch (error) {
                                  handleServerError(
                                    error,
                                    t('Failed to delete')
                                  )
                                }
                              }}
                            >
                              {t('Delete')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>

            <TabsContent value='ledger' className='mt-4'>
              <div className='rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('Kind')}</TableHead>
                      <TableHead>{t('Delta')}</TableHead>
                      <TableHead>{t('USD')}</TableHead>
                      <TableHead>{t('Remark')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(ledger.data?.items ?? []).length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={5}>{t('No ledger rows')}</TableCell>
                      </TableRow>
                    ) : (
                      (ledger.data?.items ?? []).map((row) => (
                        <TableRow key={row.id}>
                          <TableCell>
                            {row.created_at
                              ? new Date(row.created_at * 1000).toLocaleString()
                              : '—'}
                          </TableCell>
                          <TableCell>{row.kind}</TableCell>
                          <TableCell>{formatQuota(row.quota_delta)}</TableCell>
                          <TableCell>{row.money_usd ?? '—'}</TableCell>
                          <TableCell>{row.remark || '—'}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>
          </Tabs>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
