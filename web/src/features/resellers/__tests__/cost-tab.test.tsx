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
import { toast } from 'sonner'
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
  profile: { default_markup: 1.5, default_cost_factor: 1.25 },
}

const priceRow = {
  reseller_id: 3,
  model_name: 'deepseek-v4-pro',
  cost_multiplier: 1.2,
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

async function openCostTab() {
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('tab', { name: 'Reseller cost (costFactor)' })
  )
  return user
}

let putSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/admin/resellers/3') {
      return { data: { success: true, data: reseller } }
    }
    if (url === '/api/admin/resellers/3/model_prices') {
      return { data: { success: true, data: [priceRow] } }
    }
    return { data: { success: true, data: [] } }
  })
  putSpy = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: priceRow } })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin reseller detail costFactor tab', () => {
  it('labels default_markup on the overview as Reseller→Customer markup', async () => {
    renderDetail()

    expect(
      await screen.findByText('Default markup (Reseller→Customer)')
    ).toBeVisible()
    expect(screen.queryByText('Markup')).not.toBeInTheDocument()
  })

  it('shows the profile default_cost_factor on the overview as Admin→Reseller costFactor', async () => {
    renderDetail()

    const label = await screen.findByText('Default costFactor (Admin→Reseller)')
    expect(label).toBeVisible()
    expect(screen.getByText('1.25')).toBeVisible()
  })

  it('saves the costFactor as cost_multiplier and resets the form', async () => {
    renderDetail()
    const user = await openCostTab()

    await user.type(screen.getByLabelText('Model'), 'gpt-5')
    const factor = screen.getByLabelText('costFactor (Admin→Reseller)')
    await user.clear(factor)
    await user.type(factor, '1.2')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(putSpy).toHaveBeenCalledWith('/api/admin/resellers/3/model_prices', {
      model_name: 'gpt-5',
      cost_multiplier: 1.2,
    })
    expect(await screen.findByLabelText('Model')).toHaveValue('')
    expect(screen.getByLabelText('costFactor (Admin→Reseller)')).toHaveValue(1)
  })

  it('rejects a costFactor of 0 without calling the API', async () => {
    const errorSpy = vi.spyOn(toast, 'error')
    renderDetail()
    const user = await openCostTab()

    await user.type(screen.getByLabelText('Model'), 'gpt-5')
    const factor = screen.getByLabelText('costFactor (Admin→Reseller)')
    await user.clear(factor)
    await user.type(factor, '0')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(errorSpy).toHaveBeenCalledWith(
      'costFactor must be a number greater than 0'
    )
    expect(putSpy).not.toHaveBeenCalled()
  })

  it('fills the form with the row values when a row is clicked', async () => {
    renderDetail()
    const user = await openCostTab()

    await user.click(
      await screen.findByRole('cell', { name: 'deepseek-v4-pro' })
    )

    expect(screen.getByLabelText('Model')).toHaveValue('deepseek-v4-pro')
    expect(screen.getByLabelText('costFactor (Admin→Reseller)')).toHaveValue(
      1.2
    )
  })
})
