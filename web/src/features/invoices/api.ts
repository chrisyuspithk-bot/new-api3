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

import type {
  ApiResponse,
  Invoice,
  InvoiceDetail,
  PageData,
  ResellerLedgerRow,
} from './types'

export async function customerListInvoices(params: {
  p?: number
  page_size?: number
  period?: string
}): Promise<ApiResponse<PageData<Invoice>>> {
  const res = await api.get('/api/user/invoices', { params })
  return res.data
}

export async function customerGetInvoice(
  id: number
): Promise<ApiResponse<InvoiceDetail>> {
  const res = await api.get(`/api/user/invoices/${id}`)
  return res.data
}

export function customerInvoiceCsvUrl(id: number): string {
  return `/api/user/invoices/${id}?format=csv`
}

export async function adminListResellerLedger(params: {
  period?: string
  kind?: string
  reseller_id?: number
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<ResellerLedgerRow>>> {
  const res = await api.get('/api/admin/reseller_ledger', { params })
  return res.data
}

export async function customerListLedger(params: {
  period?: string
  kind?: string
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<ResellerLedgerRow>>> {
  const res = await api.get('/api/user/ledger', { params })
  return res.data
}
