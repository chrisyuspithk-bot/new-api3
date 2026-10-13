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
import { render, screen, within } from '@testing-library/react'
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
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/reseller/ledger') {
      return { data: { success: true, data: { items: ledgerRows, total: 2 } } }
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
      await screen.findByRole('cell', { name: 'Customer credit' })
    ).toBeVisible()
    expect(
      screen.queryByRole('cell', { name: 'customer_credit' })
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
})
