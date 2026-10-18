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

import { Badge } from '@/components/ui/badge'

export type BillingMode = 'markup' | 'direct_credit'

export function normalizeBillingMode(mode?: string | null): BillingMode {
  return mode === 'direct_credit' ? 'direct_credit' : 'markup'
}

/** Mode badge for customer lists; "markup" is the default. */
export function BillingModeBadge({ mode }: { mode?: string | null }) {
  const { t } = useTranslation()
  const m = normalizeBillingMode(mode)
  return (
    <Badge
      variant={m === 'direct_credit' ? 'default' : 'outline'}
      data-testid='billing-mode-badge'
    >
      {m === 'direct_credit' ? t('Direct credit') : t('Per-call markup')}
    </Badge>
  )
}

/**
 * Billing mode selector. Switching is only accepted by the server while the
 * customer balance is 0; pass balance to disable the control otherwise.
 */
export function BillingModeSelect({
  id,
  value,
  onChange,
  balance,
}: {
  id?: string
  value: BillingMode
  onChange: (mode: BillingMode) => void
  balance?: number
}) {
  const { t } = useTranslation()
  const locked = balance != null && balance !== 0
  return (
    <div className='space-y-1'>
      <select
        id={id}
        aria-label={t('Billing mode')}
        className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm disabled:opacity-60'
        value={value}
        disabled={locked}
        onChange={(e) => onChange(normalizeBillingMode(e.target.value))}
      >
        <option value='markup'>{t('Per-call markup')}</option>
        <option value='direct_credit'>{t('Direct credit')}</option>
      </select>
      <p className='text-muted-foreground text-xs'>
        {locked
          ? t('Billing mode can only be changed when the customer balance is 0')
          : value === 'direct_credit'
            ? t(
                'Direct credit: calls are billed at base price; you pay credit ÷ markup when topping up',
              )
            : t('Per-call markup: each call is billed at your markup')}
      </p>
    </div>
  )
}
