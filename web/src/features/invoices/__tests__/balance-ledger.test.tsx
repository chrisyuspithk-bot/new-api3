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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AdminInvoicesPage } from '@/features/invoices'
import { CustomerInvoicesPage } from '@/features/invoices/customer-page'
import { ResellerInvoicesPage } from '@/features/reseller-console/components/invoices-page'
import { api } from '@/lib/api'

function ledgerResponse(
  items: Array<Record<string, unknown>> = [],
  total = items.length
) {
  return { data: { success: true, data: { items, total } } }
}

function csvResponse() {
  return { data: new Blob(['# SUMMARY\n'], { type: 'text/csv' }) }
}

function stubFileSave() {
  // jsdom has no object URLs; stub the browser boundary only.
  const urlApi = URL as unknown as Record<string, unknown>
  urlApi.createObjectURL = vi.fn(() => 'blob:ledger')
  urlApi.revokeObjectURL = vi.fn()
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {})
  return { click }
}

function renderPage(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin invoices page', () => {
  beforeEach(() => {
    vi.spyOn(api, 'get').mockResolvedValue(ledgerResponse())
  })

  it('exports the selected month and kind as an authenticated CSV download', async () => {
    const { click } = stubFileSave()
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    const month = screen.getByRole('textbox', { name: 'Month' })
    await user.clear(month)
    await user.type(month, '2026-08')
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'Kind' }),
      'admin_credit'
    )
    vi.mocked(api.get).mockResolvedValueOnce(csvResponse())

    await user.click(screen.getByRole('button', { name: 'Export' }))

    await waitFor(() => expect(click).toHaveBeenCalled())
    expect(api.get).toHaveBeenCalledWith('/api/admin/reseller_ledger/export', {
      params: { period: '2026-08', kind: 'admin_credit' },
      responseType: 'blob',
    })
  })

  it('disables export while the month is not a valid YYYY-MM period', async () => {
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    const month = screen.getByRole('textbox', { name: 'Month' })
    expect(screen.getByRole('button', { name: 'Export' })).toBeEnabled()
    await user.clear(month)
    await user.type(month, '2026')
    expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()
  })

  it('loads the balance ledger for the current month and omits the invoice list and generate', async () => {
    renderPage(<AdminInvoicesPage />)
    const month = screen.getByRole('textbox', { name: 'Month' })
    expect(screen.getByRole('heading', { name: 'Invoices' })).toBeVisible()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Generate' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Reseller' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Customer' })).toBeVisible()
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/admin/reseller_ledger',
        expect.objectContaining({
          params: expect.objectContaining({
            period: (month as HTMLInputElement).value,
            p: 1,
            page_size: 20,
          }),
        })
      )
    })
    expect(await screen.findByText('No ledger rows')).toBeVisible()
  })

  it('asks for a YYYY-MM period and does not request the ledger when the month is invalid', async () => {
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    const month = screen.getByRole('textbox', { name: 'Month' })
    await waitFor(() => expect(api.get).toHaveBeenCalled())
    vi.mocked(api.get).mockClear()
    await user.clear(month)
    await user.type(month, '2026')
    expect(
      screen.getByText('Enter a period as YYYY-MM to load the balance ledger')
    ).toBeVisible()
    expect(api.get).not.toHaveBeenCalled()
  })

  it('refetches the selected month and kind on the next page', async () => {
    vi.mocked(api.get).mockResolvedValue(
      ledgerResponse(
        [
          {
            id: 4,
            reseller_id: 9,
            reseller_username: 'north-reseller',
            customer_id: 3,
            customer_username: 'ada',
            kind: 'admin_credit',
            quota_delta: 10,
            money_usd: 1.5,
            operator_id: 1,
            operator_username: 'root',
            remark: 'opening',
            created_at: 1_700_000_000,
          },
        ],
        21
      )
    )
    const user = userEvent.setup()
    renderPage(<AdminInvoicesPage />)
    expect(await screen.findByText('north-reseller')).toBeVisible()
    expect(
      within(screen.getByRole('table')).getByText('Admin credit')
    ).toBeVisible()
    const month = screen.getByRole('textbox', { name: 'Month' })
    await user.clear(month)
    await user.type(month, '2026-08')
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/admin/reseller_ledger',
        expect.objectContaining({
          params: expect.objectContaining({ period: '2026-08', p: 1 }),
        })
      )
    })
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'Kind' }),
      'customer_debit'
    )
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/admin/reseller_ledger',
        expect.objectContaining({
          params: expect.objectContaining({
            period: '2026-08',
            kind: 'customer_debit',
            p: 1,
          }),
        })
      )
    })
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/admin/reseller_ledger',
        expect.objectContaining({
          params: expect.objectContaining({
            period: '2026-08',
            kind: 'customer_debit',
            p: 2,
          }),
        })
      )
    })
  })
})

describe('role ledger pages', () => {
  beforeEach(() => {
    vi.spyOn(api, 'get').mockResolvedValue(ledgerResponse())
  })

  it('loads the reseller ledger, including customer rows, for the signed-in reseller', async () => {
    renderPage(<ResellerInvoicesPage />)
    const month = screen.getByRole('textbox', { name: 'Month' })
    expect(
      within(screen.getByRole('combobox', { name: 'Kind' })).getByRole(
        'option',
        { name: 'Admin credit' }
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Reseller' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Customer' })).toBeVisible()
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/reseller/ledger',
        expect.objectContaining({
          params: expect.objectContaining({
            period: (month as HTMLInputElement).value,
          }),
        })
      )
    })
    const params = vi.mocked(api.get).mock.calls[0]?.[1] as {
      params?: Record<string, unknown>
    }
    expect(params.params).not.toHaveProperty('reseller_id')
    expect(params.params).not.toHaveProperty('customer_id')
  })

  it.each([
    ['reseller', ResellerInvoicesPage, '/api/reseller/ledger/export'],
    ['customer', CustomerInvoicesPage, '/api/user/ledger/export'],
  ])(
    'exports the %s month through its own scoped endpoint without id params',
    async (_role, Page, exportUrl) => {
      const { click } = stubFileSave()
      const user = userEvent.setup()
      renderPage(<Page />)
      const month = screen.getByRole('textbox', { name: 'Month' })
      vi.mocked(api.get).mockResolvedValue(csvResponse())

      await user.click(screen.getByRole('button', { name: 'Export' }))

      await waitFor(() => expect(click).toHaveBeenCalled())
      expect(api.get).toHaveBeenCalledWith(exportUrl, {
        params: { period: (month as HTMLInputElement).value },
        responseType: 'blob',
      })
    }
  )

  it('loads only the signed-in customer ledger and hides other customers', async () => {
    renderPage(<CustomerInvoicesPage />)
    const kinds = screen.getByRole('combobox', { name: 'Kind' })
    expect(
      within(kinds).queryByRole('option', { name: 'Admin credit' })
    ).not.toBeInTheDocument()
    expect(
      within(kinds).getByRole('option', { name: 'Customer credit' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Customer' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Reseller' })).toBeVisible()
    expect(
      screen.queryByRole('columnheader', { name: 'Operator' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Remark' })
    ).not.toBeInTheDocument()
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/user/ledger',
        expect.objectContaining({
          params: expect.objectContaining({ p: 1, page_size: 20 }),
        })
      )
    })
    const params = vi.mocked(api.get).mock.calls[0]?.[1] as {
      params?: Record<string, unknown>
    }
    expect(params.params).not.toHaveProperty('customer_id')
    expect(params.params).not.toHaveProperty('reseller_id')
  })
})
