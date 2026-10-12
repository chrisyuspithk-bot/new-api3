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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { creditReseller, debitReseller } from '../api'
import type { Reseller } from '../types'
import { useResellers } from './resellers-provider'

type Props = {
  open: boolean
  mode: 'credit' | 'debit'
  onOpenChange: (open: boolean) => void
  currentRow: Reseller | null
}

export function ResellersQuotaDialog({
  open,
  mode,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation()
  const { triggerRefresh } = useResellers()
  const [moneyUsd, setMoneyUsd] = useState('')
  const [remark, setRemark] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!currentRow) return
    setSubmitting(true)
    try {
      const body = { money_usd: Number(moneyUsd), remark }
      if (mode === 'credit') {
        requireServerSuccess(await creditReseller(currentRow.id, body))
        toast.success(t('Reseller credited'))
      } else {
        requireServerSuccess(await debitReseller(currentRow.id, body))
        toast.success(t('Reseller debited'))
      }
      triggerRefresh()
      onOpenChange(false)
      setMoneyUsd('')
      setRemark('')
    } catch (error) {
      handleServerError(error, t('Failed to adjust reseller balance'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={onSubmit}>
          <DialogHeader>
            <DialogTitle>
              {mode === 'credit' ? t('Credit Reseller') : t('Debit Reseller')}
            </DialogTitle>
            <DialogDescription>
              {currentRow
                ? `${currentRow.username} · ${t('1 USD = cost balance')}`
                : ''}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label>{t('Amount (USD)')}</Label>
              <Input
                type='number'
                step='0.000001'
                min='0.000001'
                value={moneyUsd}
                onChange={(e) => setMoneyUsd(e.target.value)}
                required
              />
            </div>
            <div className='space-y-2'>
              <Label>{t('Remark')}</Label>
              <Input
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={submitting}>
              {submitting ? t('Saving...') : t('Confirm')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
