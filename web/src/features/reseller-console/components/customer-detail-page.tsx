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
import { Check, Copy, Loader2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  createCustomerToken,
  deleteCustomerModelMarkup,
  deleteCustomerToken,
  fetchCustomerTokenKey,
  getCustomer,
  listCustomerModelMarkups,
  listCustomerTokens,
  updateCustomer,
  upsertCustomerModelMarkup,
} from '../api'

function formatApiKey(raw?: string | null) {
  if (!raw) return ''
  return raw.startsWith('sk-') ? raw : `sk-${raw}`
}

export function ResellerCustomerDetailPage({ id }: { id: number }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [tokenName, setTokenName] = useState('default')
  const [tokenGroup, setTokenGroup] = useState('')
  const [modelName, setModelName] = useState('')
  const [modelMarkup, setModelMarkup] = useState('1.2')
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const [displayName, setDisplayName] = useState<string | null>(null)
  const [group, setGroup] = useState<string | null>(null)
  const [markup, setMarkup] = useState<string | null>(null)
  const [deleteTokenId, setDeleteTokenId] = useState<number | null>(null)
  const [deletingToken, setDeletingToken] = useState(false)
  const [copyingTokenId, setCopyingTokenId] = useState<number | null>(null)
  const [copiedTokenId, setCopiedTokenId] = useState<number | null>(null)

  const customer = useQuery({
    queryKey: ['reseller-customer', id],
    queryFn: async () => {
      const res = requireServerSuccess(await getCustomer(id))
      return res.data ?? null
    },
  })
  const tokens = useQuery({
    queryKey: ['reseller-customer-tokens', id],
    queryFn: async () => {
      const res = requireServerSuccess(
        await listCustomerTokens(id, { page_size: 50 })
      )
      return res.data ?? null
    },
  })
  const markups = useQuery({
    queryKey: ['reseller-customer-markups', id],
    queryFn: async () => {
      const res = requireServerSuccess(await listCustomerModelMarkups(id))
      return res.data ?? []
    },
  })

  const c = customer.data

  const saveProfile = async () => {
    if (!c) return
    try {
      requireServerSuccess(
        await updateCustomer(id, {
          display_name: displayName ?? c.display_name,
          group: group ?? c.group,
          markup:
            markup === '' || markup == null
              ? undefined
              : Number(markup ?? c.markup ?? undefined),
          clear_markup: markup === '',
        })
      )
      toast.success(t('Customer updated'))
      qc.invalidateQueries({ queryKey: ['reseller-customer', id] })
    } catch (error) {
      handleServerError(error, t('Failed to update customer'))
    }
  }

  const onDeleteToken = async () => {
    if (deleteTokenId == null) return
    setDeletingToken(true)
    try {
      requireServerSuccess(await deleteCustomerToken(id, deleteTokenId))
      setDeleteTokenId(null)
      qc.invalidateQueries({ queryKey: ['reseller-customer-tokens', id] })
    } catch (error) {
      handleServerError(error, t('Failed to delete token'))
    } finally {
      setDeletingToken(false)
    }
  }

  const onCopyTokenKey = useCallback(
    async (tokenId: number) => {
      setCopyingTokenId(tokenId)
      try {
        const res = requireServerSuccess(
          await fetchCustomerTokenKey(id, tokenId)
        )
        const fullKey = formatApiKey(res.data?.key)
        if (!fullKey) {
          toast.error(t('Failed to copy'))
          return
        }
        const ok = await copyToClipboard(fullKey)
        if (ok) {
          setCopiedTokenId(tokenId)
          toast.success(t('Copied!'))
          window.setTimeout(() => {
            setCopiedTokenId((cur) => (cur === tokenId ? null : cur))
          }, 2000)
        } else {
          toast.error(t('Failed to copy'))
        }
      } catch (error) {
        handleServerError(error, t('Failed to copy'))
      } finally {
        setCopyingTokenId((cur) => (cur === tokenId ? null : cur))
      }
    },
    [id, t]
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {c ? `${t('Customer')}: ${c.username}` : t('Customer')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          render={<Link to='/reseller/customers' />}
        >
          {t('Back')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!c ? (
          <div>{t('Loading...')}</div>
        ) : (
          <div className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle className='text-base'>{t('Profile')}</CardTitle>
              </CardHeader>
              <CardContent className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1.5'>
                  <Label>{t('Display Name')}</Label>
                  <Input
                    value={displayName ?? c.display_name}
                    onChange={(e) => setDisplayName(e.target.value)}
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Group')}</Label>
                  <Input
                    value={group ?? c.group}
                    onChange={(e) => setGroup(e.target.value)}
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Markup override')}</Label>
                  <Input
                    value={markup ?? (c.markup != null ? String(c.markup) : '')}
                    onChange={(e) => setMarkup(e.target.value)}
                    placeholder={t('Leave empty to inherit')}
                  />
                </div>
                <div className='flex items-end'>
                  <Button size='sm' onClick={saveProfile}>
                    {t('Save')}
                  </Button>
                </div>
                <div className='text-muted-foreground text-sm sm:col-span-2'>
                  {t('Quota')}: {formatQuota(c.quota)} · {t('Used')}:{' '}
                  {formatQuota(c.used_quota)}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='text-base'>{t('API Keys')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3'>
                <form
                  className='flex flex-wrap items-end gap-3'
                  onSubmit={async (e) => {
                    e.preventDefault()
                    try {
                      const res = requireServerSuccess(
                        await createCustomerToken(id, {
                          name: tokenName,
                          group: tokenGroup,
                          unlimited_quota: true,
                        })
                      )
                      setCreatedKey(
                        res.data?.key ? formatApiKey(res.data.key) : null
                      )
                      toast.success(t('Token created'))
                      qc.invalidateQueries({
                        queryKey: ['reseller-customer-tokens', id],
                      })
                    } catch (error) {
                      handleServerError(error, t('Failed to create token'))
                    }
                  }}
                >
                  <div className='space-y-1.5'>
                    <Label>{t('Name')}</Label>
                    <Input
                      value={tokenName}
                      onChange={(e) => setTokenName(e.target.value)}
                      required
                    />
                  </div>
                  <div className='space-y-1.5'>
                    <Label>{t('Group')}</Label>
                    <Input
                      value={tokenGroup}
                      placeholder={t('Leave empty to use default')}
                      onChange={(e) => setTokenGroup(e.target.value)}
                    />
                  </div>
                  <Button type='submit' size='sm'>
                    {t('Create Key')}
                  </Button>
                </form>
                {createdKey && (
                  <div className='bg-muted flex items-start gap-2 rounded-md p-3 text-sm'>
                    <div className='min-w-0 flex-1 break-all'>
                      {t('Copy this key now')}: {createdKey}
                    </div>
                    <Button
                      type='button'
                      size='icon'
                      variant='ghost'
                      className='size-7 shrink-0'
                      aria-label={t('Copy API key')}
                      onClick={async () => {
                        const ok = await copyToClipboard(createdKey)
                        if (ok) toast.success(t('Copied!'))
                        else toast.error(t('Failed to copy'))
                      }}
                    >
                      <Copy className='size-3.5' />
                    </Button>
                  </div>
                )}
                <div className='rounded-lg border'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('ID')}</TableHead>
                        <TableHead>{t('Name')}</TableHead>
                        <TableHead>{t('Key')}</TableHead>
                        <TableHead>{t('Group')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(tokens.data?.items ?? []).map((tok) => {
                        const maskedKey = formatApiKey(tok.key)
                        const isCopying = copyingTokenId === tok.id
                        const isCopied = copiedTokenId === tok.id
                        return (
                          <TableRow key={tok.id}>
                            <TableCell>{tok.id}</TableCell>
                            <TableCell>{tok.name}</TableCell>
                            <TableCell>
                              <div className='flex max-w-full min-w-0 items-center gap-1'>
                                <code className='truncate font-mono text-xs'>
                                  {maskedKey || '—'}
                                </code>
                                {tok.key ? (
                                  <Tooltip>
                                    <TooltipTrigger
                                      render={
                                        <Button
                                          type='button'
                                          size='icon'
                                          variant='ghost'
                                          className='size-7 shrink-0'
                                          disabled={isCopying}
                                          aria-label={t('Copy API key')}
                                          onClick={() => onCopyTokenKey(tok.id)}
                                        />
                                      }
                                    >
                                      {isCopying ? (
                                        <Loader2 className='size-3.5 animate-spin' />
                                      ) : isCopied ? (
                                        <Check className='size-3.5 text-green-600' />
                                      ) : (
                                        <Copy className='size-3.5' />
                                      )}
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      {isCopying
                                        ? t('Loading...')
                                        : isCopied
                                          ? t('Copied!')
                                          : t('Copy API key')}
                                    </TooltipContent>
                                  </Tooltip>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell>{tok.group}</TableCell>
                            <TableCell>
                              {tok.status === 1 ? t('Enabled') : t('Disabled')}
                            </TableCell>
                            <TableCell>
                              <Button
                                size='sm'
                                variant='ghost'
                                onClick={() => setDeleteTokenId(tok.id)}
                              >
                                {t('Delete')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='text-base'>
                  {t('Per-model Markups')}
                </CardTitle>
              </CardHeader>
              <CardContent className='space-y-3'>
                <form
                  className='flex flex-wrap items-end gap-3'
                  onSubmit={async (e) => {
                    e.preventDefault()
                    try {
                      requireServerSuccess(
                        await upsertCustomerModelMarkup(id, {
                          model_name: modelName,
                          markup: Number(modelMarkup),
                        })
                      )
                      toast.success(t('Markup saved'))
                      setModelName('')
                      qc.invalidateQueries({
                        queryKey: ['reseller-customer-markups', id],
                      })
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
                      value={modelMarkup}
                      onChange={(e) => setModelMarkup(e.target.value)}
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
                      {(markups.data ?? []).map((row) => (
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
                                    await deleteCustomerModelMarkup(
                                      id,
                                      row.model_name
                                    )
                                  )
                                  qc.invalidateQueries({
                                    queryKey: ['reseller-customer-markups', id],
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
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          </div>
        )}
        <ConfirmDialog
          open={deleteTokenId != null}
          onOpenChange={(o) => !o && setDeleteTokenId(null)}
          title={t('Are you sure?')}
          desc={t(
            'Are you sure you want to delete this key? This action cannot be undone.'
          )}
          confirmText={t('Delete')}
          destructive
          isLoading={deletingToken}
          handleConfirm={onDeleteToken}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
