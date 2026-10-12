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

import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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

import { listResellerCustomers } from '../api'

const PAGE_SIZE = 20

export function ResellerCustomersTab(props: { resellerId: number }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')

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

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className='space-y-3'>
      <Input
        placeholder={t('Search username or display name')}
        value={keyword}
        onChange={(e) => {
          setKeyword(e.target.value)
          setPage(1)
        }}
        className='max-w-sm'
      />
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
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell colSpan={8}>{t('Loading...')}</TableCell>
              </TableRow>
            )}
            {!isLoading && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={8}>{t('No customers')}</TableCell>
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
    </div>
  )
}
