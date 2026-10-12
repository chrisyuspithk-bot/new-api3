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

import { deleteModelMarkup, listModelMarkups, upsertModelMarkup } from '../api'

export function ResellerMarkupsPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [modelName, setModelName] = useState('')
  const [markup, setMarkup] = useState('1.2')

  const { data } = useQuery({
    queryKey: ['reseller-model-markups'],
    queryFn: async () => {
      const res = requireServerSuccess(await listModelMarkups())
      return res.data ?? []
    },
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Model Markups')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <form
          className='mb-4 flex flex-wrap items-end gap-3'
          onSubmit={async (e) => {
            e.preventDefault()
            try {
              requireServerSuccess(
                await upsertModelMarkup({
                  model_name: modelName,
                  markup: Number(markup),
                })
              )
              toast.success(t('Markup saved'))
              setModelName('')
              qc.invalidateQueries({ queryKey: ['reseller-model-markups'] })
            } catch (error) {
              handleServerError(error, t('Failed to save markup'))
            }
          }}
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
            <Label>{t('Markup')}</Label>
            <Input
              type='number'
              step='0.0001'
              min='0.0001'
              value={markup}
              onChange={(e) => setMarkup(e.target.value)}
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
                <TableHead>{t('Markup')}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell colSpan={3}>{t('No model markups')}</TableCell>
                </TableRow>
              ) : (
                (data ?? []).map((row) => (
                  <TableRow key={row.model_name}>
                    <TableCell>{row.model_name}</TableCell>
                    <TableCell>{row.markup}</TableCell>
                    <TableCell>
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={async () => {
                          try {
                            requireServerSuccess(
                              await deleteModelMarkup(row.model_name)
                            )
                            qc.invalidateQueries({
                              queryKey: ['reseller-model-markups'],
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
  )
}
