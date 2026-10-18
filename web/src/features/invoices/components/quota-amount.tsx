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
import { useTranslation } from 'react-i18next'

import { formatQuotaPrecise, formatRawQuota } from '@/lib/format'

/**
 * Money amount with full precision; the raw quota is always in the tooltip
 * and, with showRaw, printed underneath so tiny calls stay visible.
 */
export function QuotaAmount({
  quota,
  showRaw = false,
}: {
  quota: number
  showRaw?: boolean
}) {
  const { t } = useTranslation()
  const raw = `${formatRawQuota(quota)} ${t('Quota')}`
  return (
    <span title={raw} data-quota={quota}>
      {formatQuotaPrecise(quota)}
      {showRaw ? (
        <span className='text-muted-foreground block text-xs'>{raw}</span>
      ) : null}
    </span>
  )
}
