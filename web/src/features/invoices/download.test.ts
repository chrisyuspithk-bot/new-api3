import { afterEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
vi.mock('@/lib/api', () => ({ api: { get: (...a: unknown[]) => get(...a) } }))

import {
  downloadInvoiceCsv,
  invoiceCsvFilename,
  ledgerExportFilename,
} from './download'

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
    await downloadInvoiceCsv(
      '/api/admin/invoices/1?format=csv',
      'invoice-1.csv'
    )
    expect(get).toHaveBeenCalledWith('/api/admin/invoices/1?format=csv', {
      responseType: 'blob',
    })
    expect(createURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
  })

  it('rejects with the server message and saves nothing when the export fails', async () => {
    const body = JSON.stringify({
      success: false,
      message: 'period must be YYYY-MM',
    })
    get.mockResolvedValue({
      data: new Blob([body], { type: 'application/json' }),
    })
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    await expect(
      downloadInvoiceCsv('/api/user/ledger/export', 'ledger.csv', {
        period: 'bad',
      })
    ).rejects.toThrow('period must be YYYY-MM')
    expect(click).not.toHaveBeenCalled()
  })

  it('builds filenames', () => {
    expect(invoiceCsvFilename(1, '2026-09')).toBe('invoice-1-2026-09.csv')
    expect(invoiceCsvFilename(2)).toBe('invoice-2.csv')
    expect(ledgerExportFilename('2026-09')).toBe('ledger-2026-09.pdf')
    expect(ledgerExportFilename('2026-09', 'admin_debit')).toBe(
      'ledger-2026-09-admin_debit.pdf'
    )
    expect(ledgerExportFilename('2026-09', undefined, 'csv')).toBe(
      'ledger-2026-09.csv'
    )
  })
})
