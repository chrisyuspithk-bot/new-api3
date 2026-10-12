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

import { SectionPageLayout } from '@/components/layout'
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
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { createRedemptions, deleteRedemption, listRedemptions } from '../api'

export function ResellerRedemptionsPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [usd, setUsd] = useState('1')
  const [count, setCount] = useState('1')
  const [created, setCreated] = useState<string[] | null>(null)

  const { data } = useQuery({
    queryKey: ['reseller-redemptions', page],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listRedemptions({ p: page, page_size: 20 })
      )
      return res.data!
    },
  })

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Redemption Codes')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => setOpen(true)}>
            {t('Create Codes')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {created && (
            <div className='bg-muted mb-4 rounded-md p-3 text-sm'>
              <div className='mb-2 font-medium'>
                {t('Created codes (copy now)')}:
              </div>
              <pre className='overflow-auto text-xs'>{created.join('\n')}</pre>
            </div>
          )}
          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('ID')}</TableHead>
                  <TableHead>{t('Name')}</TableHead>
                  <TableHead>{t('Quota')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data?.items ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5}>{t('No codes')}</TableCell>
                  </TableRow>
                ) : (
                  (data?.items ?? []).map((row) => (
                    <TableRow key={row.id}>
                      <TableCell>{row.id}</TableCell>
                      <TableCell>{row.name}</TableCell>
                      <TableCell>{formatQuota(row.quota)}</TableCell>
                      <TableCell>{row.status}</TableCell>
                      <TableCell>
                        <Button
                          size='sm'
                          variant='ghost'
                          onClick={async () => {
                            try {
                              requireServerSuccess(
                                await deleteRedemption(row.id)
                              )
                              qc.invalidateQueries({
                                queryKey: ['reseller-redemptions'],
                              })
                            } catch (error) {
                              handleServerError(error, t('Failed to delete'))
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
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <form
            onSubmit={async (e) => {
              e.preventDefault()
              try {
                const res = requireServerSuccess(
                  await createRedemptions({
                    name,
                    quota: parseQuotaFromDollars(Number(usd)),
                    count: Number(count),
                  })
                )
                setCreated(res.data ?? [])
                toast.success(t('Codes created'))
                setOpen(false)
                qc.invalidateQueries({ queryKey: ['reseller-redemptions'] })
              } catch (error) {
                handleServerError(error, t('Failed to create codes'))
              }
            }}
          >
            <DialogHeader>
              <DialogTitle>{t('Create Codes')}</DialogTitle>
            </DialogHeader>
            <div className='grid gap-3 py-4'>
              <div className='space-y-1.5'>
                <Label>{t('Name')}</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                />
              </div>
              <div className='space-y-1.5'>
                <Label>{t('Amount (USD)')}</Label>
                <Input
                  type='number'
                  step='0.000001'
                  min='0.000001'
                  value={usd}
                  onChange={(e) => setUsd(e.target.value)}
                  required
                />
              </div>
              <div className='space-y-1.5'>
                <Label>{t('Count')}</Label>
                <Input
                  type='number'
                  min='1'
                  value={count}
                  onChange={(e) => setCount(e.target.value)}
                  required
                />
              </div>
            </div>
            <DialogFooter>
              <Button type='submit'>{t('Create')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
