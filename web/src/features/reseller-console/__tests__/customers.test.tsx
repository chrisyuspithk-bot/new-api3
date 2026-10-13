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

import { ResellerCustomersPage } from '@/features/reseller-console/components/customers-page'
import { api } from '@/lib/api'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: (props: { children?: ReactNode }) => <a href='/'>{props.children}</a>,
}))

const customer = {
  id: 7,
  username: 'demodemo3',
  display_name: 'Demo',
  status: 1,
  group: 'vip',
  quota: 0,
  used_quota: 0,
  request_count: 0,
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ResellerCustomersPage />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/reseller/customers') {
      return { data: { success: true, data: { items: [customer], total: 1 } } }
    }
    return {
      data: {
        success: true,
        data: { id: 9, username: 'r', display_name: '', quota: 50000000 },
      },
    }
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: {} },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('reseller customers page', () => {
  it('creates a customer with an empty group so the server picks an allowed group', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Add Customer' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(
      within(dialog).getByRole('textbox', { name: 'Username' }),
      'newcust'
    )
    await user.type(within(dialog).getByLabelText('Password'), 'password123')

    await user.click(within(dialog).getByRole('button', { name: 'Create' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/reseller/customers',
        expect.objectContaining({ username: 'newcust', group: '' })
      )
    )
  })

  it('credits the named customer with the entered amount and remark', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Credit' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/demodemo3/)).toBeVisible()
    await user.type(
      within(dialog).getByRole('spinbutton', { name: 'Amount (USD)' }),
      '5'
    )
    await user.type(
      within(dialog).getByRole('textbox', { name: 'Remark' }),
      'October top-up'
    )

    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/reseller/customers/7/credit',
        {
          money_usd: 5,
          remark: 'October top-up',
        }
      )
    )
  })

  it('deletes a customer only after confirming in the dialog', async () => {
    vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('demodemo3')).toBeVisible()
    expect(api.delete).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(api.delete).toHaveBeenCalledWith('/api/reseller/customers/7')
    )
  })

  it('keeps the customer when the delete confirmation is cancelled', async () => {
    vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
    expect(api.delete).not.toHaveBeenCalled()
  })
})
