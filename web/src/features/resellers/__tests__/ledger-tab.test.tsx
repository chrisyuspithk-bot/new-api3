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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ResellerDetail } from '@/features/resellers/components/reseller-detail'
import { api } from '@/lib/api'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: (props: { children?: ReactNode }) => <a href='/'>{props.children}</a>,
}))

const reseller = {
  id: 3,
  username: 'agent1',
  display_name: '',
  status: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
}

const ledgerRow = {
  id: 1,
  kind: 'admin_credit',
  quota_delta: 1250000,
  money_usd: 2.5,
  customer_id: 0,
  operator_id: 1,
  operator_username: 'root',
  remark: '',
  created_at: 1791849600,
}

type LedgerResponse = { success: boolean; message?: string; data?: unknown }
type LedgerParams = { p?: number; page_size?: number }

function mockApi(
  ledger: (params: LedgerParams) => Promise<{ data: LedgerResponse }>
) {
  return vi
    .spyOn(api, 'get')
    .mockImplementation(
      async (url: string, config?: { params?: LedgerParams }) => {
        if (url === '/api/admin/resellers/3/ledger') {
          return ledger(config?.params ?? {})
        }
        if (url === '/api/admin/resellers/3') {
          return { data: { success: true, data: reseller } }
        }
        return { data: { success: true, data: [] } }
      }
    )
}

function renderDetail() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ResellerDetail id={3} />
    </QueryClientProvider>
  )
}

async function openLedgerTab() {
  const user = userEvent.setup()
  await user.click(await screen.findByRole('tab', { name: 'Ledger' }))
  return user
}

beforeEach(() => {
  mockApi(async () => ({
    data: { success: true, data: { items: [ledgerRow], total: 1 } },
  }))
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin reseller detail ledger tab', () => {
  it('formats the USD amount with a dollar sign and four decimals', async () => {
    renderDetail()
    await openLedgerTab()

    expect(await screen.findByRole('cell', { name: '$2.5000' })).toBeVisible()
  })

  it('shows a load error instead of the empty state when the ledger request fails', async () => {
    mockApi(async () => ({ data: { success: false, message: 'boom' } }))
    renderDetail()
    await openLedgerTab()

    expect(await screen.findByText('Failed to load')).toBeVisible()
    expect(screen.queryByText('No ledger rows')).not.toBeInTheDocument()
  })

  it('shows a loading row instead of the empty state while the ledger loads', async () => {
    mockApi(() => new Promise(() => {}))
    renderDetail()
    await openLedgerTab()

    expect(
      await screen.findByRole('cell', { name: 'Loading...' })
    ).toBeVisible()
    expect(screen.queryByText('No ledger rows')).not.toBeInTheDocument()
  })

  it('disables Previous and Next on page 1 when the total fits one page', async () => {
    renderDetail()
    await openLedgerTab()

    expect(await screen.findByText('1 / 1')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })

  it('requests page 2 with page size 20 when Next is clicked and total exceeds one page', async () => {
    const ledger = vi.fn(async (params: LedgerParams) => ({
      data: {
        success: true,
        data: {
          items: [
            { ...ledgerRow, id: params.p ?? 1, remark: `page ${params.p}` },
          ],
          total: 45,
        },
      },
    }))
    mockApi(ledger)
    renderDetail()
    const user = await openLedgerTab()

    expect(await screen.findByText('Total: 45')).toBeVisible()
    expect(screen.getByText('1 / 3')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(ledger).toHaveBeenLastCalledWith({ p: 1, page_size: 20 })

    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(await screen.findByRole('cell', { name: 'page 2' })).toBeVisible()
    expect(ledger).toHaveBeenLastCalledWith({ p: 2, page_size: 20 })
    expect(screen.getByText('2 / 3')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  })
})
