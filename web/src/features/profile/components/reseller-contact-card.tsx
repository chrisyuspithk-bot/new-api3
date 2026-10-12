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
import { Store } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'

import type { UserProfile } from '../types'

export function ResellerContactCard({
  profile,
}: {
  profile: UserProfile | null
}) {
  const { t } = useTranslation()
  if (!profile?.reseller || profile.reseller.role !== 'customer') {
    return null
  }
  const name =
    profile.reseller.reseller_display_name ||
    t('Your reseller')

  return (
    <Card>
      <CardHeader className='flex flex-row items-center gap-3 space-y-0'>
        <IconBadge tone='info'>
          <Store className='size-4' />
        </IconBadge>
        <CardTitle className='text-base'>{t('Reseller Contact')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-2 text-sm'>
        <div>
          <span className='text-muted-foreground'>{t('Provider')}: </span>
          <span className='font-medium'>{name}</span>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Online top-up is disabled for reseller customers. Contact your reseller for quota or redemption codes.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
