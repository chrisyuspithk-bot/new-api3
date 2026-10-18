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
  Customer,
  CustomerToken,
  GrossProfitRow,
  LedgerRow,
  ModelMarkup,
  PageData,
  ResellerLogRow,
  ResellerRedemption,
  ResellerSelf,
} from './types'

export async function getResellerSelf(): Promise<ApiResponse<ResellerSelf>> {
  const res = await api.get('/api/reseller/self')
  return res.data
}

export async function updateResellerMarkup(
  default_markup: number
): Promise<ApiResponse<{ default_markup: number }>> {
  const res = await api.put('/api/reseller/self/markup', { default_markup })
  return res.data
}

export async function listCustomers(params: {
  p?: number
  page_size?: number
  keyword?: string
}): Promise<ApiResponse<PageData<Customer>>> {
  const res = await api.get('/api/reseller/customers', { params })
  return res.data
}

export async function getCustomer(id: number): Promise<ApiResponse<Customer>> {
  const res = await api.get(`/api/reseller/customers/${id}`)
  return res.data
}

export async function createCustomer(body: {
  username: string
  password: string
  display_name?: string
  group?: string
  markup?: number
  remark?: string
  billing_mode?: 'markup' | 'direct_credit'
}): Promise<ApiResponse<Customer>> {
  const res = await api.post('/api/reseller/customers', body)
  return res.data
}

export async function updateCustomer(
  id: number,
  body: {
    display_name?: string
    password?: string
    group?: string
    markup?: number
    clear_markup?: boolean
    remark?: string
    billing_mode?: 'markup' | 'direct_credit'
  }
): Promise<ApiResponse<Customer>> {
  const res = await api.put(`/api/reseller/customers/${id}`, body)
  return res.data
}

export async function setCustomerStatus(
  id: number,
  status: number
): Promise<ApiResponse<{ status: number }>> {
  const res = await api.post(`/api/reseller/customers/${id}/status`, {
    status,
  })
  return res.data
}

export async function deleteCustomer(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/reseller/customers/${id}`)
  return res.data
}

export async function creditCustomer(
  id: number,
  body: { quota?: number; money_usd?: number; remark?: string }
): Promise<ApiResponse<{ quota_delta: number }>> {
  const res = await api.post(`/api/reseller/customers/${id}/credit`, body)
  return res.data
}

export async function debitCustomer(
  id: number,
  body: { quota?: number; money_usd?: number; remark?: string }
): Promise<ApiResponse<{ quota_delta: number }>> {
  const res = await api.post(`/api/reseller/customers/${id}/debit`, body)
  return res.data
}

export async function listCustomerTokens(
  id: number,
  params: { p?: number; page_size?: number } = {}
): Promise<ApiResponse<PageData<CustomerToken>>> {
  const res = await api.get(`/api/reseller/customers/${id}/tokens`, { params })
  return res.data
}

export async function createCustomerToken(
  id: number,
  body: {
    name: string
    group?: string
    remain_quota?: number
    unlimited_quota?: boolean
    expired_time?: number
  }
): Promise<ApiResponse<CustomerToken>> {
  const res = await api.post(`/api/reseller/customers/${id}/tokens`, body)
  return res.data
}

export async function updateCustomerToken(
  id: number,
  tokenId: number,
  body: Record<string, unknown>
): Promise<ApiResponse<CustomerToken>> {
  const res = await api.put(
    `/api/reseller/customers/${id}/tokens/${tokenId}`,
    body
  )
  return res.data
}

export async function deleteCustomerToken(
  id: number,
  tokenId: number
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/reseller/customers/${id}/tokens/${tokenId}`
  )
  return res.data
}

/** Fetch the full (unmasked) customer token key. List responses stay masked. */
export async function fetchCustomerTokenKey(
  customerId: number,
  tokenId: number
): Promise<ApiResponse<{ key: string }>> {
  const res = await api.post(
    `/api/reseller/customers/${customerId}/tokens/${tokenId}/key`
  )
  return res.data
}

export async function listModelMarkups(): Promise<ApiResponse<ModelMarkup[]>> {
  const res = await api.get('/api/reseller/model_markups')
  return res.data
}

export async function upsertModelMarkup(body: {
  model_name: string
  markup: number
}): Promise<ApiResponse<ModelMarkup>> {
  const res = await api.put('/api/reseller/model_markups', body)
  return res.data
}

export async function deleteModelMarkup(model: string): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/reseller/model_markups/${encodeURIComponent(model)}`
  )
  return res.data
}

export async function listCustomerModelMarkups(
  id: number
): Promise<ApiResponse<ModelMarkup[]>> {
  const res = await api.get(`/api/reseller/customers/${id}/model_markups`)
  return res.data
}

export async function upsertCustomerModelMarkup(
  id: number,
  body: { model_name: string; markup: number }
): Promise<ApiResponse<ModelMarkup>> {
  const res = await api.put(`/api/reseller/customers/${id}/model_markups`, body)
  return res.data
}

export async function deleteCustomerModelMarkup(
  id: number,
  model: string
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/reseller/customers/${id}/model_markups/${encodeURIComponent(model)}`
  )
  return res.data
}

export async function listRedemptions(params: {
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<ResellerRedemption>>> {
  const res = await api.get('/api/reseller/redemptions', { params })
  return res.data
}

export async function createRedemptions(body: {
  name: string
  quota: number
  count?: number
  expired_time?: number
}): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/reseller/redemptions', body)
  return res.data
}

export async function deleteRedemption(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/reseller/redemptions/${id}`)
  return res.data
}

export async function listResellerLogs(params: {
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<ResellerLogRow>>> {
  const res = await api.get('/api/reseller/logs', { params })
  return res.data
}

export async function getGrossProfit(params: {
  from?: number
  to?: number
  group_by?: 'model' | 'customer'
}): Promise<ApiResponse<GrossProfitRow[]>> {
  const res = await api.get('/api/reseller/reports/gross_profit', { params })
  return res.data
}

export async function getLedger(params: {
  p?: number
  page_size?: number
  period?: string
  kind?: string
}): Promise<ApiResponse<PageData<LedgerRow>>> {
  const res = await api.get('/api/reseller/ledger', { params })
  return res.data
}
