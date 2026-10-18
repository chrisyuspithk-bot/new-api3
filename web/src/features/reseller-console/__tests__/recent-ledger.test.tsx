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

import { ResellerOverview } from '@/features/reseller-console'
import { api } from '@/lib/api'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: (props: { children?: ReactNode }) => <a href='/'>{props.children}</a>,
}))

const ledgerRows = [
  {
    id: 2,
    kind: 'customer_credit',
    quota_delta: 2500000,
    customer_id: 7,
    customer_username: 'demodemo3',
    remark: '',
    created_at: 1791849600,
  },
  {
    id: 1,
    kind: 'admin_credit',
    quota_delta: 5000000,
    customer_id: 0,
    remark: '',
    created_at: 1791849000,
  },
]

function renderOverview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ResellerOverview />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(
    async (
      url: string,
      config?: { params?: { p?: number; page_size?: number } },
    ) => {
      if (url === '/api/reseller/ledger') {
        const page = config?.params?.p ?? 1
        const pageSize = config?.params?.page_size ?? 20
        // 25 total so paging is exercisable; page 1 returns fixture rows.
        const items =
          page === 1
            ? ledgerRows
            : [
                {
                  id: 3,
                  kind: 'admin_debit',
                  quota_delta: -100,
                  customer_id: 0,
                  remark: 'page2',
                  created_at: 1791848000,
                },
              ]
        return {
          data: {
            success: true,
            data: { items, total: 25, page, page_size: pageSize },
          },
        }
      }
      return {
        data: {
          success: true,
          data: {
            id: 9,
            username: 'r',
            display_name: '',
            quota: 0,
            used_quota: 0,
          },
        },
      }
    },
  )
})

afterEach(() => {
  vi.restoreAllMocks()
})

function customerCell(row: HTMLElement): HTMLElement {
  const headers = screen.getAllByRole('columnheader')
  const index = headers.findIndex((h) => h.textContent === 'Customer')
  return within(row).getAllByRole('cell')[index]
}

describe('reseller overview recent ledger', () => {
  it('shows the customer username instead of the numeric customer id', async () => {
    renderOverview()
    await screen.findAllByRole('cell', {
      name: /customer_credit|Customer credit/,
    })
    const rows = screen.getAllByRole('row').slice(1)
    expect(customerCell(rows[0])).toHaveTextContent(/^demodemo3$/)
  })

  it('shows a dash in the customer column when the row has no customer', async () => {
    renderOverview()
    await screen.findAllByRole('cell', {
      name: /customer_credit|Customer credit/,
    })
    const rows = screen.getAllByRole('row').slice(1)
    expect(customerCell(rows[1])).toHaveTextContent(/^—$/)
  })

  it('shows the translated kind label instead of the raw kind', async () => {
    renderOverview()
    expect(
      await screen.findByRole('cell', { name: 'Customer credit' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('cell', { name: 'customer_credit' }),
    ).not.toBeInTheDocument()
  })

  it('shows a load error instead of the empty state when the ledger request fails', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url === '/api/reseller/ledger') {
        return { data: { success: false, message: 'boom' } }
      }
      return {
        data: {
          success: true,
          data: {
            id: 9,
            username: 'r',
            display_name: '',
            quota: 0,
            used_quota: 0,
          },
        },
      }
    })
    renderOverview()
    expect(await screen.findByText('Failed to load')).toBeVisible()
    expect(screen.queryByText('No ledger rows')).not.toBeInTheDocument()
  })
  it('pages the recent ledger with Total, Previous and Next', async () => {
    const user = userEvent.setup()
    renderOverview()
    expect(await screen.findByText('Total: 25')).toBeVisible()
    expect(screen.getByText('1 / 2')).toBeVisible()
    const prev = screen.getByRole('button', { name: 'Previous' })
    const next = screen.getByRole('button', { name: 'Next' })
    expect(prev).toBeDisabled()
    expect(next).toBeEnabled()

    await user.click(next)
    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/reseller/ledger',
        expect.objectContaining({
          params: expect.objectContaining({ p: 2, page_size: 20 }),
        }),
      )
    })
    expect(await screen.findByText('2 / 2')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByText('page2')).toBeVisible()
  })
})
