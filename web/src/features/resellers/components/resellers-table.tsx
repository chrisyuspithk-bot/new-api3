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
import { Link } from '@tanstack/react-router'
import { MoreHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { deleteReseller, listResellers } from '../api'
import { useResellers } from './resellers-provider'

export function ResellersTable() {
  const { t } = useTranslation()
  const { refreshTrigger, triggerRefresh, setOpen, setCurrentRow } =
    useResellers()
  const [page, setPage] = useState(1)
  const pageSize = 20

  const { data, isLoading } = useQuery({
    queryKey: ['admin-resellers', page, pageSize, refreshTrigger],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listResellers({ p: page, page_size: pageSize })
      )
      return res.data!
    },
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const handleDelete = async (id: number) => {
    if (!window.confirm(t('Disable this reseller and freeze its customers?'))) {
      return
    }
    try {
      requireServerSuccess(await deleteReseller(id))
      toast.success(t('Reseller disabled'))
      triggerRefresh()
    } catch (error) {
      handleServerError(error, t('Failed to delete reseller'))
    }
  }

  return (
    <div className='space-y-3'>
      <div className='rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('ID')}</TableHead>
              <TableHead>{t('Username')}</TableHead>
              <TableHead>{t('Brand')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Balance')}</TableHead>
              <TableHead>{t('Markup')}</TableHead>
              <TableHead>{t('Customers')}</TableHead>
              <TableHead>{t('Groups')}</TableHead>
              <TableHead className='w-12' />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={9}>{t('Loading...')}</TableCell>
              </TableRow>
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={9}>{t('No resellers yet')}</TableCell>
              </TableRow>
            ) : (
              items.map((row) => (
                <TableRow key={row.id}>
                  <TableCell>{row.id}</TableCell>
                  <TableCell>
                    <Link
                      to='/resellers/$id'
                      params={{ id: String(row.id) }}
                      className='font-medium text-primary hover:underline'
                    >
                      {row.username}
                    </Link>
                    <div className='text-muted-foreground text-xs'>
                      {row.display_name}
                    </div>
                  </TableCell>
                  <TableCell>
                    {row.profile?.display_name || row.display_name || '—'}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      variant={row.status === 1 ? 'success' : 'neutral'}
                      label={row.status === 1 ? t('Enabled') : t('Disabled')}
                      copyable={false}
                    />
                  </TableCell>
                  <TableCell>{formatQuota(row.quota)}</TableCell>
                  <TableCell>
                    {row.profile?.default_markup?.toFixed?.(4) ?? '—'}
                  </TableCell>
                  <TableCell>{row.customer_count ?? 0}</TableCell>
                  <TableCell className='max-w-[160px] truncate text-xs'>
                    {(row.groups ?? []).join(', ') || '—'}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={<Button variant='ghost' size='icon' />}
                      >
                        <MoreHorizontal className='h-4 w-4' />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='end'>
                        <DropdownMenuItem
                          onClick={() => {
                            setCurrentRow(row)
                            setOpen('update')
                          }}
                        >
                          {t('Edit')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => {
                            setCurrentRow(row)
                            setOpen('credit')
                          }}
                        >
                          {t('Credit')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => {
                            setCurrentRow(row)
                            setOpen('debit')
                          }}
                        >
                          {t('Debit')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => handleDelete(row.id)}
                          className='text-destructive'
                        >
                          {t('Disable')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      <div className='text-muted-foreground flex items-center justify-between text-sm'>
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
    </div>
  )
}
