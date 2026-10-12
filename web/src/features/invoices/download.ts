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
import { api } from '@/lib/api'

import type { Invoice } from './types'

/**
 * Downloads an invoice CSV through the authenticated API client.
 *
 * Dashboard auth is a Bearer token held by the SPA, so a plain
 * `<a href>` navigation carries no Authorization header and the server
 * answers AUTH_UNAUTHORIZED. Fetch the CSV as a blob instead and save it.
 */
export async function downloadInvoiceCsv(
  url: string,
  filename: string
): Promise<void> {
  const res = await api.get(url, { responseType: 'blob' })
  const data = res.data as Blob
  const blob =
    data instanceof Blob ? data : new Blob([String(data)], { type: 'text/csv' })
  saveBlobAsFile(blob, filename)
}

export function invoiceCsvFilename(id: number, period?: string): string {
  return period ? `invoice-${id}-${period}.csv` : `invoice-${id}.csv`
}

export function invoicesListCsvFilename(period: string): string {
  return `invoices-${period}.csv`
}

function csvEscape(value: string | number | null | undefined): string {
  const s = value == null ? '' : String(value)
  if (/[",\n\r]/.test(s)) {
    return `"${s.replace(/"/g, '""')}"`
  }
  return s
}

/** Build a list-level CSV from already-loaded invoice rows (client-side). */
export function buildInvoicesListCsv(rows: Invoice[]): string {
  const headers = [
    'id',
    'period',
    'scope',
    'reseller_id',
    'reseller_name',
    'customer_id',
    'customer_name',
    'requests',
    'charge_quota',
    'cost_quota',
    'profit_quota',
    'charge_usd',
    'cost_usd',
    'profit_usd',
    'generated_at',
    'generated_by',
  ]
  const lines = [headers.join(',')]
  for (const row of rows) {
    lines.push(
      [
        row.id,
        row.period,
        row.scope,
        row.reseller_id,
        row.reseller_name,
        row.customer_id,
        row.customer_name,
        row.requests,
        row.charge_quota,
        row.cost_quota,
        row.profit_quota,
        row.charge_usd,
        row.cost_usd,
        row.profit_usd,
        row.generated_at,
        row.generated_by,
      ]
        .map(csvEscape)
        .join(',')
    )
  }
  return lines.join('\n') + '\n'
}

/** Download the currently filtered invoice list as CSV (client-side). */
export function downloadInvoicesListCsv(
  rows: Invoice[],
  filename: string
): void {
  const csv = buildInvoicesListCsv(rows)
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  saveBlobAsFile(blob, filename)
}

function saveBlobAsFile(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob)
  try {
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = filename
    link.rel = 'noopener'
    document.body.appendChild(link)
    link.click()
    link.remove()
  } finally {
    setTimeout(() => URL.revokeObjectURL(objectUrl), 0)
  }
}
