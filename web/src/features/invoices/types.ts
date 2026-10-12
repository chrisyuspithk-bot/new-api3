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
export type InvoiceScope = 'customer' | 'reseller' | 'admin_rollup'

export interface Invoice {
  id: number
  period: string
  scope: InvoiceScope
  reseller_id: number
  customer_id: number
  reseller_name: string
  customer_name: string
  requests: number
  charge_quota: number
  cost_quota: number
  profit_quota: number
  charge_usd: number
  cost_usd: number
  profit_usd: number
  line_items_json?: string
  generated_at: number
  generated_by: number
}

export interface InvoiceLineItem {
  model: string
  requests: number
  charge_quota: number
  cost_quota: number
  profit_quota: number
  charge_usd: number
  cost_usd: number
  profit_usd: number
}

export interface InvoiceDetail {
  invoice: Invoice
  line_items: InvoiceLineItem[]
}

export interface InvoiceGenerateResult {
  period: string
  customer_invoices: number
  reseller_invoices: number
  admin_rollup: boolean
  from_unix: number
  to_unix: number
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data?: T
}

export interface PageData<T> {
  items?: T[]
  total?: number
  page?: number
  page_size?: number
}

export type ResellerLedgerKind =
  | 'admin_credit'
  | 'admin_debit'
  | 'customer_credit'
  | 'customer_debit'

export interface ResellerLedgerRow {
  id: number
  reseller_id: number
  reseller_username?: string
  customer_id: number
  customer_username?: string
  kind: string
  quota_delta: number
  money_usd?: number | null
  operator_id: number
  operator_username?: string
  remark?: string
  created_at: number
}
