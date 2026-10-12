import { afterEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
vi.mock('@/lib/api', () => ({ api: { get: (...a: unknown[]) => get(...a) } }))

import { downloadInvoiceCsv, invoiceCsvFilename } from './download'

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
  })
})
