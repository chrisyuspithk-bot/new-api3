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
import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { usePricingData } from '../hooks/use-pricing-data'

const getPricing = vi.fn()

vi.mock('../api', () => ({
  getPricing: () => getPricing(),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { price: 1, usd_exchange_rate: 1 } }),
}))

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
  }
}

describe('usePricingData auth-scoped cache', () => {
  beforeEach(() => {
    getPricing.mockReset()
    useAuthStore.getState().auth.reset('idle')
  })

  it('refetches after login so customer markup replaces anonymous base prices', async () => {
    getPricing
      .mockResolvedValueOnce({
        success: true,
        data: [
          {
            model_name: 'deepseek-v4-pro',
            model_ratio: 37.5,
            model_price: 0,
          },
        ],
        vendors: [],
        group_ratio: { default: 1 },
        usable_group: { default: 'Default' },
        supported_endpoint: {},
        auto_groups: [],
      })
      .mockResolvedValueOnce({
        success: true,
        data: [
          {
            model_name: 'deepseek-v4-pro',
            model_ratio: 112.5,
            model_price: 0,
          },
        ],
        vendors: [],
        group_ratio: { default: 1 },
        usable_group: { default: 'Default' },
        supported_endpoint: {},
        auto_groups: [],
      })

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    useAuthStore.getState().auth.setBootstrapState('complete')
    const { result, rerender } = renderHook(() => usePricingData(), {
      wrapper: wrapper(client),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.models[0]?.model_ratio).toBe(37.5)
    expect(getPricing).toHaveBeenCalledTimes(1)

    useAuthStore.getState().auth.setBundle({
      access_token: 'tok',
      token_type: 'Bearer',
      access_expires_at: Date.now() / 1000 + 3600,
      user: {
        id: 7,
        username: 'demodemo3',
        role: 1,
        reseller_id: 6,
      },
      session: {
        sid: 's1',
        current: true,
        login_method: 'password',
        ip: '',
        user_agent: '',
        created_at: 0,
        last_active_at: 0,
        expires_at: 0,
      },
    })
    rerender()

    await waitFor(() =>
      expect(result.current.models[0]?.model_ratio).toBe(112.5)
    )
    expect(getPricing).toHaveBeenCalledTimes(2)
  })

  it('waits for auth bootstrap before fetching', () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    useAuthStore.getState().auth.reset('idle')
    renderHook(() => usePricingData(), { wrapper: wrapper(client) })
    expect(getPricing).not.toHaveBeenCalled()
  })
})
