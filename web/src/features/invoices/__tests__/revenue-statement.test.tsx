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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AdminInvoicesPage } from '@/features/invoices'
import { ResellerInvoicesPage } from '@/features/reseller-console/components/invoices-page'
import { api } from '@/lib/api'

const agg = (over: Record<string, unknown> = {}) => ({
  key: '0',
  id: 0,
  name: '',
  requests: 0,
  refunds: 0,
  customer_price: 0,
  reseller_cost: 0,
  reseller_profit: 0,
  admin_revenue: 0,
  base_quota_micro: 0,
  admin_profit_micro: 0,
  flagged_rows: 0,
  ...over,
})

function statement(scope: 'admin' | 'reseller') {
  return {
    period: '2026-10',
    timezone: 'Asia/Hong_Kong',
    scope,
    reseller_id: scope === 'reseller' ? 6 : undefined,
    reseller_name: scope === 'reseller' ? 'demodemo2' : undefined,
    totals: agg({ requests: 2, customer_price: 129, reseller_cost: 116 }),
    customer_total: agg({
      requests: 1,
      customer_price: 81,
      reseller_cost: 68,
      reseller_profit: 13,
    }),
    self_total: agg({ requests: 1, customer_price: 48, reseller_cost: 48 }),
    direct_total: agg(),
    by_reseller: [
      agg({
        key: '6',
        id: 6,
        name: 'demodemo2',
        requests: 1,
        customer_price: 81,
      }),
    ],
    reseller_self: [agg({ key: '6', id: 6, name: 'demodemo2', requests: 1 })],
    direct: [],
    by_customer: [
      agg({ key: '15', id: 15, name: 'SPIT', requests: 1, customer_price: 81 }),
    ],
    reconciliation: {
      identity_ok: true,
      identity_diff_micro: 0,
      sections_ok: true,
      log_compared: true,
      log_charge_quota: 129,
      log_reseller_cost: 68,
      log_charge_diff: 0,
      log_reseller_cost_diff: 0,
      flagged_rows: 0,
      pending_retry: 0,
    },
    row_count: 2,
    checksum: 'abc',
    frozen: false,
    late_rows: 0,
    checksum_mismatch: false,
    generated_at: 1792320000,
  }
}

function renderPage(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

afterEach(() => vi.restoreAllMocks())

function mockApi(scope: 'admin' | 'reseller') {
  return vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url.includes('/revenue/statement')) {
      return { data: { success: true, data: statement(scope) } }
    }
    return { data: { success: true, data: { items: [], total: 0 } } }
  })
}

describe('revenue tab', () => {
  it('admin sees per-reseller, self-use, direct sections and reconciliation', async () => {
    const get = mockApi('admin')
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    await user.click(screen.getByRole('tab', { name: 'Revenue' }))
    const page = await screen.findByTestId('revenue-statement')
    expect(get).toHaveBeenCalledWith(
      '/api/admin/revenue/statement',
      expect.objectContaining({
        params: expect.objectContaining({ period: expect.any(String) }),
      })
    )
    expect(within(page).getByText('Customer sales by reseller')).toBeVisible()
    expect(
      within(page).getAllByText('Reseller self-use').length
    ).toBeGreaterThan(0)
    expect(within(page).getByText('Direct users')).toBeVisible()
    expect(
      within(page).getByText('Charge = base + admin margin + reseller profit')
    ).toBeVisible()
    expect(within(page).getAllByText('Admin margin').length).toBeGreaterThan(0)
  })

  it('reseller sees per-customer sales without admin margin', async () => {
    mockApi('reseller')
    const user = userEvent.setup()
    renderPage(<ResellerInvoicesPage />)
    await user.click(screen.getByRole('tab', { name: 'Revenue' }))
    const page = await screen.findByTestId('revenue-statement')
    expect(within(page).getByText('Sales by customer')).toBeVisible()
    expect(within(page).getByText('SPIT')).toBeVisible()
    expect(within(page).queryByText('Admin margin')).not.toBeInTheDocument()
    expect(within(page).queryByText('Base price')).not.toBeInTheDocument()
  })

  it('exports the revenue statement as PDF and CSV', async () => {
    const get = mockApi('admin')
    const urlApi = URL as unknown as Record<string, unknown>
    urlApi.createObjectURL = vi.fn(() => 'blob:x')
    urlApi.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    await user.click(screen.getByRole('tab', { name: 'Revenue' }))
    await screen.findByTestId('revenue-statement')
    get.mockResolvedValue({
      data: new Blob(['%PDF'], { type: 'application/pdf' }),
    })
    await user.click(screen.getByRole('button', { name: 'Export' }))
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith(
        '/api/admin/revenue/statement',
        expect.objectContaining({
          params: expect.objectContaining({ format: 'pdf' }),
          responseType: 'blob',
        })
      )
    )
    await user.click(screen.getByRole('button', { name: 'Export CSV' }))
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith(
        '/api/admin/revenue/statement',
        expect.objectContaining({
          params: expect.objectContaining({ format: 'csv' }),
        })
      )
    )
  })
})
