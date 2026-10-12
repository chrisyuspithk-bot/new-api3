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
  GrossProfitRow,
  PageData,
  ReconcileIssue,
  Reseller,
  ResellerCustomer,
  ResellerFormData,
  ResellerLedgerRow,
  ResellerModelPrice,
} from './types'

export async function listResellers(params: {
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<Reseller>>> {
  const res = await api.get('/api/admin/resellers', { params })
  return res.data
}

export async function getReseller(id: number): Promise<ApiResponse<Reseller>> {
  const res = await api.get(`/api/admin/resellers/${id}`)
  return res.data
}

export async function createReseller(
  data: ResellerFormData
): Promise<ApiResponse<Reseller>> {
  const res = await api.post('/api/admin/resellers', data)
  return res.data
}

export async function updateReseller(
  id: number,
  data: ResellerFormData
): Promise<ApiResponse<Reseller>> {
  const res = await api.put(`/api/admin/resellers/${id}`, data)
  return res.data
}

export async function deleteReseller(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/admin/resellers/${id}`)
  return res.data
}

export async function creditReseller(
  id: number,
  body: { quota?: number; money_usd?: number; remark?: string }
): Promise<ApiResponse<{ quota_delta: number }>> {
  const res = await api.post(`/api/admin/resellers/${id}/credit`, body)
  return res.data
}

export async function debitReseller(
  id: number,
  body: { quota?: number; money_usd?: number; remark?: string }
): Promise<ApiResponse<{ quota_delta: number }>> {
  const res = await api.post(`/api/admin/resellers/${id}/debit`, body)
  return res.data
}

export async function getResellerGroups(
  id: number
): Promise<ApiResponse<string[]>> {
  const res = await api.get(`/api/admin/resellers/${id}/groups`)
  return res.data
}

export async function setResellerGroups(
  id: number,
  groups: string[]
): Promise<ApiResponse<string[]>> {
  const res = await api.put(`/api/admin/resellers/${id}/groups`, { groups })
  return res.data
}

export async function listResellerModelPrices(
  id: number
): Promise<ApiResponse<ResellerModelPrice[]>> {
  const res = await api.get(`/api/admin/resellers/${id}/model_prices`)
  return res.data
}

export async function upsertResellerModelPrice(
  id: number,
  body: ResellerModelPrice
): Promise<ApiResponse<ResellerModelPrice>> {
  const res = await api.put(`/api/admin/resellers/${id}/model_prices`, body)
  return res.data
}

export async function deleteResellerModelPrice(
  id: number,
  model: string
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/admin/resellers/${id}/model_prices/${encodeURIComponent(model)}`
  )
  return res.data
}

export async function getResellerLedger(
  id: number,
  params: { p?: number; page_size?: number } = {}
): Promise<ApiResponse<PageData<ResellerLedgerRow>>> {
  const res = await api.get(`/api/admin/resellers/${id}/ledger`, { params })
  return res.data
}

export async function listResellerCustomers(
  id: number,
  params: { p?: number; page_size?: number; keyword?: string } = {}
): Promise<ApiResponse<PageData<ResellerCustomer>>> {
  const res = await api.get(`/api/admin/resellers/${id}/customers`, { params })
  return res.data
}

export async function getGrossProfit(params: {
  reseller_id?: number
  from?: number
  to?: number
  group_by?: 'model' | 'customer'
}): Promise<ApiResponse<GrossProfitRow[]>> {
  const res = await api.get('/api/admin/reports/gross_profit', { params })
  return res.data
}

export async function getResellerReconcile(params: {
  from?: number
}): Promise<ApiResponse<{ issues: ReconcileIssue[]; count: number }>> {
  const res = await api.get('/api/admin/reports/reseller_reconcile', {
    params,
  })
  return res.data
}
