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
  InvoiceGenerateResult,
  PageData,
} from './types'

export async function adminListInvoices(params: {
  p?: number
  page_size?: number
  period?: string
  scope?: string
  reseller_id?: number
  customer_id?: number
}): Promise<ApiResponse<PageData<Invoice>>> {
  const res = await api.get('/api/admin/invoices', { params })
  return res.data
}

export async function adminGetInvoice(
  id: number
): Promise<ApiResponse<InvoiceDetail>> {
  const res = await api.get(`/api/admin/invoices/${id}`)
  return res.data
}

export async function adminGenerateInvoices(
  period: string
): Promise<ApiResponse<InvoiceGenerateResult>> {
  const res = await api.post('/api/admin/invoices/generate', { period })
  return res.data
}

export function adminInvoiceCsvUrl(id: number): string {
  return `/api/admin/invoices/${id}?format=csv`
}

export async function resellerListInvoices(params: {
  p?: number
  page_size?: number
  period?: string
  scope?: string
  customer_id?: number
}): Promise<ApiResponse<PageData<Invoice>>> {
  const res = await api.get('/api/reseller/invoices', { params })
  return res.data
}

export async function resellerGetInvoice(
  id: number
): Promise<ApiResponse<InvoiceDetail>> {
  const res = await api.get(`/api/reseller/invoices/${id}`)
  return res.data
}

export function resellerInvoiceCsvUrl(id: number): string {
  return `/api/reseller/invoices/${id}?format=csv`
}

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
