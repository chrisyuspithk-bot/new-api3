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
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ResellersMutateDrawer } from '@/features/resellers/components/resellers-mutate-drawer'
import { ResellersProvider } from '@/features/resellers/components/resellers-provider'
import type { Reseller } from '@/features/resellers/types'
import { api } from '@/lib/api'

const existing: Reseller = {
  id: 3,
  username: 'agent1',
  display_name: 'Agent',
  status: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
  groups: ['default'],
  profile: {
    user_id: 3,
    default_markup: 1.5,
    default_cost_factor: 1.3,
    max_customers: 0,
    allow_redemption: false,
    display_name: 'Agent',
  },
}

function renderDrawer(currentRow?: Reseller) {
  return render(
    <ResellersProvider>
      <ResellersMutateDrawer
        open
        onOpenChange={() => {}}
        currentRow={currentRow}
      />
    </ResellersProvider>
  )
}

let postSpy: ReturnType<typeof vi.spyOn>
let putSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  postSpy = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: existing } })
  putSpy = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: existing } })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('reseller mutate drawer default_cost_factor', () => {
  it('defaults the cost factor to 1 on create and submits it', async () => {
    const user = userEvent.setup()
    renderDrawer()

    expect(screen.getByLabelText('Default Cost Factor')).toHaveValue(1)
    await user.type(screen.getByLabelText('Username'), 'agent2')
    await user.type(screen.getByLabelText('Password'), 'password123')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(postSpy).toHaveBeenCalledWith(
      '/api/admin/resellers',
      expect.objectContaining({ default_cost_factor: 1, default_markup: 1.2 })
    )
  })

  it('loads the saved cost factor on edit and submits the changed value', async () => {
    const user = userEvent.setup()
    renderDrawer(existing)

    const field = screen.getByLabelText('Default Cost Factor')
    expect(field).toHaveValue(1.3)
    await user.clear(field)
    await user.type(field, '0.9')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(putSpy).toHaveBeenCalledWith(
      '/api/admin/resellers/3',
      expect.objectContaining({ default_cost_factor: 0.9 })
    )
  })
})
