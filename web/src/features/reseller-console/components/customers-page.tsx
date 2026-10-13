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
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getCurrencyDisplay } from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  createCustomer,
  creditCustomer,
  debitCustomer,
  deleteCustomer,
  getResellerSelf,
  listCustomers,
  setCustomerStatus,
} from '../api'
import type { Customer } from '../types'

export function ResellerCustomersPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [adjustTarget, setAdjustTarget] = useState<{
    customer: Customer
    mode: 'credit' | 'debit'
  } | null>(null)
  const [form, setForm] = useState({
    username: '',
    password: '',
    display_name: '',
    group: '',
    markup: '',
  })
  const [adjustUsd, setAdjustUsd] = useState('')
  const [adjustRemark, setAdjustRemark] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Customer | null>(null)
  const [deleting, setDeleting] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['reseller-customers', page, keyword],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listCustomers({ p: page, page_size: 20, keyword })
      )
      return res.data ?? null
    },
  })

  const { data: self } = useQuery({
    queryKey: ['reseller-self'],
    queryFn: async () => {
      const res = requireServerSuccess(await getResellerSelf())
      return res.data ?? null
    },
  })

  const availableCostQuota = self?.quota ?? 0
  const quotaPerUnit = (() => {
    const { config } = getCurrencyDisplay()
    return config.quotaPerUnit > 0 ? config.quotaPerUnit : 500000
  })()
  const isDebit = adjustTarget?.mode === 'debit'
  // Credit draws on the reseller cost balance; debit claws back from the customer.
  const adjustLimitQuota = isDebit
    ? (adjustTarget?.customer.quota ?? 0)
    : availableCostQuota
  const adjustLimitUsd = adjustLimitQuota / quotaPerUnit
  const adjustAmountUsd = Number(adjustUsd)
  const adjustExceedsBalance =
    Number.isFinite(adjustAmountUsd) &&
    adjustAmountUsd > 0 &&
    adjustAmountUsd > adjustLimitUsd + 1e-12
  const adjustExceedsMessage = isDebit
    ? t('Amount exceeds customer balance')
    : t('Amount exceeds available cost balance')

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['reseller-customers'] })
    qc.invalidateQueries({ queryKey: ['reseller-self'] })
    qc.invalidateQueries({ queryKey: ['reseller-ledger'] })
  }

  const onCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      requireServerSuccess(
        await createCustomer({
          ...form,
          markup: form.markup ? Number(form.markup) : undefined,
        })
      )
      toast.success(t('Customer created'))
      setCreateOpen(false)
      setForm({
        username: '',
        password: '',
        display_name: '',
        group: '',
        markup: '',
      })
      refresh()
    } catch (error) {
      handleServerError(error, t('Failed to create customer'))
    }
  }

  const closeAdjust = () => {
    setAdjustTarget(null)
    setAdjustUsd('')
    setAdjustRemark('')
  }

  const onAdjust = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!adjustTarget) return
    if (adjustExceedsBalance) {
      toast.error(adjustExceedsMessage)
      return
    }
    const body = { money_usd: Number(adjustUsd), remark: adjustRemark }
    try {
      if (isDebit) {
        requireServerSuccess(
          await debitCustomer(adjustTarget.customer.id, body)
        )
        toast.success(t('Customer debited'))
      } else {
        requireServerSuccess(
          await creditCustomer(adjustTarget.customer.id, body)
        )
        toast.success(t('Customer credited'))
      }
      closeAdjust()
      refresh()
    } catch (error) {
      handleServerError(
        error,
        isDebit ? t('Failed to debit customer') : t('Failed to credit customer')
      )
    }
  }

  const onDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      requireServerSuccess(await deleteCustomer(deleteTarget.id))
      toast.success(t('Customer deleted'))
      setDeleteTarget(null)
      refresh()
    } catch (error) {
      handleServerError(error, t('Failed to delete customer'))
    } finally {
      setDeleting(false)
    }
  }

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / 20))

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Customers')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => setCreateOpen(true)}>
            <Plus className='h-4 w-4' />
            {t('Add Customer')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mb-3 flex gap-2'>
            <Input
              placeholder={t('Search username or display name')}
              value={keyword}
              onChange={(e) => {
                setKeyword(e.target.value)
                setPage(1)
              }}
              className='max-w-sm'
            />
          </div>
          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('ID')}</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead>{t('Quota')}</TableHead>
                  <TableHead>{t('Markup')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading && (
                  <TableRow>
                    <TableCell colSpan={7}>{t('Loading...')}</TableCell>
                  </TableRow>
                )}
                {!isLoading && items.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={7}>{t('No customers')}</TableCell>
                  </TableRow>
                )}
                {items.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell>{row.id}</TableCell>
                    <TableCell>
                      <Link
                        to='/reseller/customers/$id'
                        params={{ id: String(row.id) }}
                        className='text-primary font-medium hover:underline'
                      >
                        {row.username}
                      </Link>
                      <div className='text-muted-foreground text-xs'>
                        {row.display_name}
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        variant={row.status === 1 ? 'success' : 'neutral'}
                        label={row.status === 1 ? t('Enabled') : t('Disabled')}
                        copyable={false}
                      />
                    </TableCell>
                    <TableCell>{row.group}</TableCell>
                    <TableCell>{formatQuota(row.quota)}</TableCell>
                    <TableCell>
                      {row.markup != null ? row.markup : t('Default')}
                    </TableCell>
                    <TableCell className='space-x-1'>
                      <Button
                        size='sm'
                        variant='outline'
                        onClick={() =>
                          setAdjustTarget({ customer: row, mode: 'credit' })
                        }
                      >
                        {t('Credit')}
                      </Button>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={row.quota <= 0}
                        onClick={() =>
                          setAdjustTarget({ customer: row, mode: 'debit' })
                        }
                      >
                        {t('Debit')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={async () => {
                          try {
                            requireServerSuccess(
                              await setCustomerStatus(
                                row.id,
                                row.status === 1 ? 2 : 1
                              )
                            )
                            refresh()
                          } catch (error) {
                            handleServerError(
                              error,
                              t('Failed to update status')
                            )
                          }
                        }}
                      >
                        {row.status === 1 ? t('Disable') : t('Enable')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        className='text-destructive'
                        onClick={() => setDeleteTarget(row)}
                      >
                        {t('Delete')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <div className='text-muted-foreground mt-3 flex items-center justify-between text-sm'>
            <span>
              {t('Total')}: {total}
            </span>
            <div className='flex gap-2'>
              <Button
                size='sm'
                variant='outline'
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
              >
                {t('Previous')}
              </Button>
              <span>
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
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <form onSubmit={onCreate}>
            <DialogHeader>
              <DialogTitle>{t('Add Customer')}</DialogTitle>
            </DialogHeader>
            <div className='grid gap-3 py-4'>
              {(
                [
                  ['username', t('Username'), true],
                  ['password', t('Password'), true],
                  ['display_name', t('Display Name'), false],
                  ['group', t('Group'), false],
                  ['markup', t('Markup override'), false],
                ] as const
              ).map(([key, label, required]) => (
                <div key={key} className='space-y-1.5'>
                  <Label htmlFor={`reseller-customer-${key}`}>{label}</Label>
                  <Input
                    id={`reseller-customer-${key}`}
                    type={key === 'password' ? 'password' : 'text'}
                    required={required}
                    placeholder={
                      key === 'group'
                        ? t('Leave empty to use default')
                        : undefined
                    }
                    value={form[key]}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, [key]: e.target.value }))
                    }
                  />
                </div>
              ))}
            </div>
            <DialogFooter>
              <Button type='submit'>{t('Create')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={adjustTarget != null}
        onOpenChange={(o) => !o && closeAdjust()}
      >
        <DialogContent>
          <form onSubmit={onAdjust}>
            <DialogHeader>
              <DialogTitle>
                {isDebit ? t('Debit Customer') : t('Credit Customer')}
              </DialogTitle>
              <DialogDescription>
                {adjustTarget?.customer.username}
              </DialogDescription>
            </DialogHeader>
            <div className='space-y-2 py-4'>
              <Label htmlFor='reseller-adjust-amount'>
                {t('Amount (USD)')}
              </Label>
              <Input
                id='reseller-adjust-amount'
                type='number'
                step='0.000001'
                min='0.000001'
                max={adjustLimitUsd > 0 ? adjustLimitUsd : undefined}
                value={adjustUsd}
                onChange={(e) => setAdjustUsd(e.target.value)}
                required
              />
              <p className='text-muted-foreground text-xs'>
                {isDebit ? t('Customer balance') : t('Available cost balance')}:{' '}
                {formatQuota(adjustLimitQuota)}
              </p>
              {adjustExceedsBalance ? (
                <p className='text-destructive text-xs'>
                  {adjustExceedsMessage}
                </p>
              ) : null}
              <Label htmlFor='reseller-adjust-remark'>{t('Remark')}</Label>
              <Input
                id='reseller-adjust-remark'
                value={adjustRemark}
                onChange={(e) => setAdjustRemark(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button type='submit' disabled={adjustExceedsBalance}>
                {t('Confirm')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget != null}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        title={t('Disable customer and revoke tokens?')}
        desc={<span className='font-semibold'>{deleteTarget?.username}</span>}
        confirmText={t('Delete')}
        destructive
        isLoading={deleting}
        handleConfirm={onDelete}
      />
    </>
  )
}
