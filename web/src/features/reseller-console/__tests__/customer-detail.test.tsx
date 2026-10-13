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

import { ResellerCustomerDetailPage } from '@/features/reseller-console/components/customer-detail-page'
import { api } from '@/lib/api'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: (props: { children?: ReactNode }) => <a href='/'>{props.children}</a>,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ResellerCustomerDetailPage id={7} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/reseller/customers/7/tokens') {
      const token = { id: 31, name: 'prod-key', status: 1, group: 'vip' }
      return { data: { success: true, data: { items: [token], total: 1 } } }
    }
    if (url === '/api/reseller/customers/7/model_markups') {
      return { data: { success: true, data: [] } }
    }
    return {
      data: {
        success: true,
        data: {
          id: 7,
          username: 'demodemo3',
          display_name: 'Demo',
          group: 'vip',
          quota: 0,
          used_quota: 0,
        },
      },
    }
  })
  vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('reseller customer detail API keys', () => {
  it('deletes an API key only after confirming in the dialog', async () => {
    const user = userEvent.setup()
    renderPage()
    const row = (await screen.findByText('prod-key')).closest('tr')
    if (!row) throw new Error('token row not rendered')
    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(api.delete).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(api.delete).toHaveBeenCalledWith(
        '/api/reseller/customers/7/tokens/31'
      )
    )
  })
})
