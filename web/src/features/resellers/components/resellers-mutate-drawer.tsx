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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { createReseller, updateReseller } from '../api'
import type { Reseller } from '../types'
import { useResellers } from './resellers-provider'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Reseller
}

export function ResellersMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useResellers()
  const [submitting, setSubmitting] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [brandName, setBrandName] = useState('')
  const [group, setGroup] = useState('default')
  const [groups, setGroups] = useState('default')
  const [markup, setMarkup] = useState('1.2')
  const [maxCustomers, setMaxCustomers] = useState('0')
  const [allowRedemption, setAllowRedemption] = useState(true)
  const [status, setStatus] = useState(1)

  useEffect(() => {
    if (!open) return
    if (currentRow) {
      setUsername(currentRow.username)
      setPassword('')
      setDisplayName(currentRow.display_name || '')
      setBrandName(currentRow.profile?.display_name || '')
      setGroup(currentRow.group || 'default')
      setGroups((currentRow.groups ?? []).join(', '))
      setMarkup(String(currentRow.profile?.default_markup ?? 1))
      setMaxCustomers(String(currentRow.profile?.max_customers ?? 0))
      setAllowRedemption(currentRow.profile?.allow_redemption ?? true)
      setStatus(currentRow.status)
    } else {
      setUsername('')
      setPassword('')
      setDisplayName('')
      setBrandName('')
      setGroup('default')
      setGroups('default')
      setMarkup('1.2')
      setMaxCustomers('0')
      setAllowRedemption(true)
      setStatus(1)
    }
  }, [open, currentRow])

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const groupList = groups
        .split(',')
        .map((g) => g.trim())
        .filter(Boolean)
      const payload = {
        display_name: displayName || username,
        group,
        groups: groupList,
        default_markup: Number(markup),
        max_customers: Number(maxCustomers),
        allow_redemption: allowRedemption,
        brand_name: brandName || displayName || username,
        status,
        ...(isUpdate
          ? password
            ? { password }
            : {}
          : { username, password }),
      }
      if (isUpdate && currentRow) {
        requireServerSuccess(await updateReseller(currentRow.id, payload))
        toast.success(t('Reseller updated'))
      } else {
        requireServerSuccess(await createReseller(payload))
        toast.success(t('Reseller created'))
      }
      triggerRefresh()
      onOpenChange(false)
    } catch (error) {
      handleServerError(
        error,
        isUpdate ? t('Failed to update reseller') : t('Failed to create reseller')
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName()}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Edit Reseller') : t('Add Reseller')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Resellers sell to their own customers with a cost balance and markup.'
            )}
          </SheetDescription>
        </SheetHeader>
        <form className={sideDrawerFormClassName()} onSubmit={onSubmit}>
          <div className='space-y-4 p-4'>
            {!isUpdate && (
              <>
                <div className='space-y-2'>
                  <Label>{t('Username')}</Label>
                  <Input
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    required
                  />
                </div>
                <div className='space-y-2'>
                  <Label>{t('Password')}</Label>
                  <Input
                    type='password'
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                </div>
              </>
            )}
            {isUpdate && (
              <div className='space-y-2'>
                <Label>{t('New Password')} ({t('optional')})</Label>
                <Input
                  type='password'
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
            )}
            <div className='space-y-2'>
              <Label>{t('Display Name')}</Label>
              <Input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label>{t('Brand Name')}</Label>
              <Input
                value={brandName}
                onChange={(e) => setBrandName(e.target.value)}
                placeholder={t('Shown to customers')}
              />
            </div>
            <div className='space-y-2'>
              <Label>{t('Default Group')}</Label>
              <Input value={group} onChange={(e) => setGroup(e.target.value)} />
            </div>
            <div className='space-y-2'>
              <Label>{t('Allowed Groups')}</Label>
              <Input
                value={groups}
                onChange={(e) => setGroups(e.target.value)}
                placeholder='default, vip'
              />
              <p className='text-muted-foreground text-xs'>
                {t('Comma-separated whitelist of groups customers may use.')}
              </p>
            </div>
            <div className='grid grid-cols-2 gap-3'>
              <div className='space-y-2'>
                <Label>{t('Default Markup')}</Label>
                <Input
                  type='number'
                  step='0.0001'
                  min='0.0001'
                  value={markup}
                  onChange={(e) => setMarkup(e.target.value)}
                  required
                />
              </div>
              <div className='space-y-2'>
                <Label>{t('Max Customers')}</Label>
                <Input
                  type='number'
                  min='0'
                  value={maxCustomers}
                  onChange={(e) => setMaxCustomers(e.target.value)}
                />
              </div>
            </div>
            <div className='flex items-center justify-between'>
              <Label>{t('Allow Redemption Codes')}</Label>
              <Switch
                checked={allowRedemption}
                onCheckedChange={setAllowRedemption}
              />
            </div>
            {isUpdate && (
              <div className='flex items-center justify-between'>
                <Label>{t('Enabled')}</Label>
                <Switch
                  checked={status === 1}
                  onCheckedChange={(v) => setStatus(v ? 1 : 2)}
                />
              </div>
            )}
          </div>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={submitting}>
              {submitting ? t('Saving...') : t('Save')}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
