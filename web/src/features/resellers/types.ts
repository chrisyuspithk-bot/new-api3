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
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface ResellerProfile {
  user_id: number
  default_markup: number
  default_cost_factor: number
  max_customers: number
  allow_redemption: boolean
  display_name: string
  created_at?: number
  updated_at?: number
}

export interface Reseller {
  id: number
  username: string
  display_name: string
  status: number
  group: string
  quota: number
  used_quota: number
  profile?: ResellerProfile
  groups?: string[]
  customer_count?: number
}

export interface ResellerCustomer {
  id: number
  username: string
  display_name: string
  status: number
  group: string
  quota: number
  used_quota: number
  request_count: number
  markup?: number | null
  remark?: string
  created_at?: number
}

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface ResellerFormData {
  username?: string
  password?: string
  display_name?: string
  user_id?: number
  group?: string
  groups?: string[]
  default_markup?: number
  default_cost_factor?: number
  max_customers?: number
  allow_redemption?: boolean
  brand_name?: string
  status?: number
}

export interface ResellerModelPrice {
  id?: number
  reseller_id?: number
  model_name: string
  model_ratio?: number | null
  model_price?: number | null
  completion_ratio?: number | null
  cost_multiplier?: number | null
  updated_at?: number
}

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
  ref_id?: string
  remark?: string
  created_at: number
}

export interface GrossProfitRow {
  key: string
  requests: number
  charge_quota: number
  cost_quota: number
  profit_quota: number
}

export interface ReconcileIssue {
  reseller_id: number
  username?: string
  kind: string
  detail: string
  value: number
}

export type ResellersDialogType =
  | 'create'
  | 'update'
  | 'credit'
  | 'debit'
  | 'delete'
  | null
