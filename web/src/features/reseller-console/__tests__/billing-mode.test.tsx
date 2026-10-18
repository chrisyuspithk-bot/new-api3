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
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  BillingModeBadge,
  BillingModeSelect,
  normalizeBillingMode,
} from '@/features/reseller-console/components/billing-mode'
import { ledgerKindLabel } from '@/features/invoices/lib/ledger-kind'

describe('billing mode', () => {
  it('defaults unknown or empty modes to markup', () => {
    expect(normalizeBillingMode(undefined)).toBe('markup')
    expect(normalizeBillingMode('')).toBe('markup')
    expect(normalizeBillingMode('bogus')).toBe('markup')
    expect(normalizeBillingMode('direct_credit')).toBe('direct_credit')
  })

  it('renders badges for both modes', () => {
    render(
      <>
        <BillingModeBadge mode='' />
        <BillingModeBadge mode='direct_credit' />
      </>,
    )
    expect(screen.getByText('Per-call markup')).toBeTruthy()
    expect(screen.getByText('Direct credit')).toBeTruthy()
  })

  it('locks the selector while the customer has a balance', () => {
    render(
      <BillingModeSelect value='markup' onChange={vi.fn()} balance={500} />,
    )
    const select = screen.getByLabelText('Billing mode') as HTMLSelectElement
    expect(select.disabled).toBe(true)
    expect(
      screen.getByText(
        'Billing mode can only be changed when the customer balance is 0',
      ),
    ).toBeTruthy()
  })

  it('labels direct credit ledger kinds', () => {
    const t = ((k: string) => k) as never
    expect(ledgerKindLabel('direct_credit_cost', t)).toBe('Direct credit cost')
    expect(ledgerKindLabel('direct_credit_admin_profit', t)).toBe(
      'Direct credit admin profit',
    )
    expect(ledgerKindLabel('direct_credit_reseller_profit', t)).toBe(
      'Direct credit reseller profit',
    )
  })
})
