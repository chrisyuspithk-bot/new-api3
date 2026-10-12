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
