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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  deleteResellerModelPrice,
  listResellerModelPrices,
  upsertResellerModelPrice,
} from '../api'
import type { ResellerModelPrice } from '../types'

// Admin→Reseller per-model cost: reseller cost = base × costFactor. The
// Reseller→Customer markup is configured separately (default_markup).
export function ResellerCostTab(props: { resellerId: number }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [modelName, setModelName] = useState('')
  const [costMultiplier, setCostMultiplier] = useState('1')

  const prices = useQuery({
    queryKey: ['admin-reseller-prices', props.resellerId],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listResellerModelPrices(props.resellerId)
      )
      return res.data ?? []
    },
  })

  const editRow = (row: ResellerModelPrice) => {
    setModelName(row.model_name)
    setCostMultiplier(
      row.cost_multiplier != null ? String(row.cost_multiplier) : ''
    )
  }

  const savePrice = async (e: React.FormEvent) => {
    e.preventDefault()
    const model = modelName.trim()
    const factor = Number(costMultiplier)
    if (!model) {
      toast.error(t('Model name is required'))
      return
    }
    if (
      costMultiplier.trim() === '' ||
      !Number.isFinite(factor) ||
      factor <= 0
    ) {
      toast.error(t('costFactor must be a number greater than 0'))
      return
    }
    try {
      requireServerSuccess(
        await upsertResellerModelPrice(props.resellerId, {
          model_name: model,
          cost_multiplier: factor,
        })
      )
      toast.success(t('costFactor saved'))
      setModelName('')
      setCostMultiplier('1')
      qc.invalidateQueries({
        queryKey: ['admin-reseller-prices', props.resellerId],
      })
    } catch (error) {
      handleServerError(error, t('Failed to save costFactor'))
    }
  }

  const removePrice = async (model: string) => {
    try {
      requireServerSuccess(
        await deleteResellerModelPrice(props.resellerId, model)
      )
      qc.invalidateQueries({
        queryKey: ['admin-reseller-prices', props.resellerId],
      })
    } catch (error) {
      handleServerError(error, t('Failed to delete'))
    }
  }

  const rows = prices.data ?? []

  return (
    <div className='space-y-4'>
      <div className='text-muted-foreground space-y-1 text-sm'>
        <p>
          {t(
            'Admin→Reseller cost per model. Reseller cost = base price × costFactor; the reseller sees this cost on /pricing and is charged it for API usage.'
          )}
        </p>
        <p>
          {t(
            'Customer charge = reseller cost × markup. Markup (Reseller→Customer) is set separately and is not a costFactor. Models without a row use the reseller Default Cost Factor (1 if unset).'
          )}
        </p>
      </div>
      <form
        onSubmit={savePrice}
        noValidate
        className='flex flex-wrap items-end gap-3'
      >
        <div className='space-y-1.5'>
          <Label htmlFor='reseller-cost-model'>{t('Model')}</Label>
          <Input
            id='reseller-cost-model'
            value={modelName}
            onChange={(e) => setModelName(e.target.value)}
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor='reseller-cost-factor'>
            {t('costFactor (Admin→Reseller)')}
          </Label>
          <Input
            id='reseller-cost-factor'
            type='number'
            step='0.0001'
            min='0.0001'
            value={costMultiplier}
            onChange={(e) => setCostMultiplier(e.target.value)}
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
              <TableHead>{t('costFactor (Admin→Reseller)')}</TableHead>
              <TableHead>{t('Ratio')}</TableHead>
              <TableHead>{t('Price')}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5}>
                  {t('No costFactor overrides')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.model_name}
                  className='cursor-pointer'
                  onClick={() => editRow(row)}
                >
                  <TableCell>{row.model_name}</TableCell>
                  <TableCell>{row.cost_multiplier ?? '—'}</TableCell>
                  <TableCell>{row.model_ratio ?? '—'}</TableCell>
                  <TableCell>{row.model_price ?? '—'}</TableCell>
                  <TableCell className='space-x-1 text-right'>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={(e) => {
                        e.stopPropagation()
                        editRow(row)
                      }}
                    >
                      {t('Edit')}
                    </Button>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={(e) => {
                        e.stopPropagation()
                        removePrice(row.model_name)
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
    </div>
  )
}
