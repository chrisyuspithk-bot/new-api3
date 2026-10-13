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
import { Plus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
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
import { getUser } from '@/features/users/api'
import { UsersDeleteDialog } from '@/features/users/components/users-delete-dialog'
import { UsersMutateDrawer } from '@/features/users/components/users-mutate-drawer'
import {
  UsersProvider,
  useUsers,
} from '@/features/users/components/users-provider'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { createResellerCustomer, listResellerCustomers } from '../api'
import type { ResellerCustomer } from '../types'

const PAGE_SIZE = 20

function ResellerCustomersTabInner(props: { resellerId: number }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({
    username: '',
    password: '',
    display_name: '',
    group: '',
    markup: '',
  })
  const { open, setOpen, currentRow, setCurrentRow, refreshTrigger } =
    useUsers()

  const { data, isLoading } = useQuery({
    queryKey: ['admin-reseller-customers', props.resellerId, page, keyword],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listResellerCustomers(props.resellerId, {
          p: page,
          page_size: PAGE_SIZE,
          keyword,
        })
      )
      return res.data
    },
  })

  useEffect(() => {
    if (refreshTrigger > 0) {
      qc.invalidateQueries({
        queryKey: ['admin-reseller-customers', props.resellerId],
      })
    }
  }, [refreshTrigger, props.resellerId, qc])

  const openUserDialog = async (
    row: ResellerCustomer,
    dialog: 'update' | 'delete'
  ) => {
    try {
      const res = requireServerSuccess(await getUser(row.id))
      if (!res.data) return
      setCurrentRow(res.data)
      setOpen(dialog)
    } catch (error) {
      handleServerError(error, t('Failed to load user'))
    }
  }

  const onCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      requireServerSuccess(
        await createResellerCustomer(props.resellerId, {
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
      qc.invalidateQueries({
        queryKey: ['admin-reseller-customers', props.resellerId],
      })
    } catch (error) {
      handleServerError(error, t('Failed to create customer'))
    }
  }

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          placeholder={t('Search username or display name')}
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value)
            setPage(1)
          }}
          className='max-w-sm'
        />
        <Button size='sm' onClick={() => setCreateOpen(true)}>
          <Plus className='h-4 w-4' />
          {t('Add Customer')}
        </Button>
      </div>
      <div className='rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('ID')}</TableHead>
              <TableHead>{t('Username')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('User Group')}</TableHead>
              <TableHead>{t('Balance')}</TableHead>
              <TableHead>{t('Used')}</TableHead>
              <TableHead>{t('Requests')}</TableHead>
              <TableHead>{t('Markup')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell colSpan={9}>{t('Loading...')}</TableCell>
              </TableRow>
            )}
            {!isLoading && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={9}>{t('No customers')}</TableCell>
              </TableRow>
            )}
            {items.map((row) => (
              <TableRow key={row.id}>
                <TableCell>{row.id}</TableCell>
                <TableCell>
                  <div className='font-medium'>{row.username}</div>
                  {row.display_name && row.display_name !== row.username && (
                    <div className='text-muted-foreground text-xs'>
                      {row.display_name}
                    </div>
                  )}
                </TableCell>
                <TableCell>
                  <StatusBadge
                    variant={row.status === 1 ? 'success' : 'neutral'}
                    label={row.status === 1 ? t('Enabled') : t('Disabled')}
                    copyable={false}
                  />
                </TableCell>
                <TableCell>
                  <GroupBadge group={row.group} className='font-normal' />
                </TableCell>
                <TableCell>{formatQuota(row.quota)}</TableCell>
                <TableCell>{formatQuota(row.used_quota)}</TableCell>
                <TableCell>{row.request_count}</TableCell>
                <TableCell>
                  {row.markup != null ? row.markup : t('Default')}
                </TableCell>
                <TableCell className='text-right'>
                  <div className='inline-flex items-center gap-1'>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => openUserDialog(row, 'update')}
                    >
                      {t('Edit')}
                    </Button>
                    <Button
                      size='sm'
                      variant='ghost'
                      className='text-destructive'
                      onClick={() => openUserDialog(row, 'delete')}
                    >
                      {t('Remove')}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
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
                  <Label htmlFor={`admin-reseller-customer-${key}`}>
                    {label}
                  </Label>
                  <Input
                    id={`admin-reseller-customer-${key}`}
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

      <UsersMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow || undefined : undefined}
      />
      <UsersDeleteDialog />
    </div>
  )
}

export function ResellerCustomersTab(props: { resellerId: number }) {
  return (
    <UsersProvider>
      <ResellerCustomersTabInner resellerId={props.resellerId} />
    </UsersProvider>
  )
}
