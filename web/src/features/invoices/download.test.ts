import { afterEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
vi.mock('@/lib/api', () => ({ api: { get: (...a: unknown[]) => get(...a) } }))

import {
  buildInvoicesListCsv,
  downloadInvoiceCsv,
  downloadInvoicesListCsv,
  invoiceCsvFilename,
  invoicesListCsvFilename,
} from './download'
import type { Invoice } from './types'

describe('downloadInvoiceCsv', () => {
  afterEach(() => vi.restoreAllMocks())

  it('fetches CSV via the authenticated api client and saves it', async () => {
    get.mockResolvedValue({ data: new Blob(['a,b\n'], { type: 'text/csv' }) })
    const createURL = vi.fn(() => 'blob:x')
    ;(URL as unknown as Record<string, unknown>).createObjectURL = createURL
    ;(URL as unknown as Record<string, unknown>).revokeObjectURL = vi.fn()
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    await downloadInvoiceCsv('/api/admin/invoices/1?format=csv', 'invoice-1.csv')
    expect(get).toHaveBeenCalledWith('/api/admin/invoices/1?format=csv', {
      responseType: 'blob',
    })
    expect(createURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
  })

  it('builds filenames', () => {
    expect(invoiceCsvFilename(1, '2026-09')).toBe('invoice-1-2026-09.csv')
    expect(invoiceCsvFilename(2)).toBe('invoice-2.csv')
    expect(invoicesListCsvFilename('2026-10')).toBe('invoices-2026-10.csv')
  })
})

describe('buildInvoicesListCsv', () => {
  const sample: Invoice = {
    id: 7,
    period: '2026-10',
    scope: 'customer',
    reseller_id: 1,
    customer_id: 2,
    reseller_name: 'R, Inc',
    customer_name: 'Cust "A"',
    requests: 3,
    charge_quota: 100,
    cost_quota: 40,
    profit_quota: 60,
    charge_usd: 1.0,
    cost_usd: 0.4,
    profit_usd: 0.6,
    generated_at: 1720000000,
    generated_by: 9,
  }

  it('emits header + escaped rows', () => {
    const csv = buildInvoicesListCsv([sample])
    const lines = csv.trimEnd().split('\n')
    expect(lines[0]).toContain('id,period,scope,reseller_id')
    expect(lines[1]).toContain('7,2026-10,customer,1')
    expect(lines[1]).toContain('"R, Inc"')
    expect(lines[1]).toContain('"Cust ""A"""')
  })

  it('downloadInvoicesListCsv triggers a file save', () => {
    const createURL = vi.fn(() => 'blob:list')
    ;(URL as unknown as Record<string, unknown>).createObjectURL = createURL
    ;(URL as unknown as Record<string, unknown>).revokeObjectURL = vi.fn()
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    downloadInvoicesListCsv([sample], 'invoices-2026-10.csv')
    expect(createURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
  })
})
