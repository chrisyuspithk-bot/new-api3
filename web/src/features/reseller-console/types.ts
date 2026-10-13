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

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface ResellerSelf {
  id: number
  username: string
  display_name: string
  status: number
  group: string
  quota: number
  used_quota: number
  profile?: {
    user_id: number
    default_markup: number
    max_customers: number
    allow_redemption: boolean
    display_name: string
  }
  groups?: string[]
  customer_count?: number
}

export interface Customer {
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

export interface CustomerToken {
  id: number
  name: string
  key?: string
  status: number
  group: string
  remain_quota: number
  unlimited_quota: boolean
  expired_time: number
  created_time?: number
}

export interface ModelMarkup {
  id?: number
  model_name: string
  markup: number
  updated_at?: number
}

export interface ResellerRedemption {
  id: number
  name: string
  key?: string
  status: number
  quota: number
  created_time?: number
  redeemed_time?: number
  expired_time?: number
}

export interface ResellerLogRow {
  id?: number
  created_at: number
  username?: string
  model_name?: string
  quota: number
  reseller_cost_quota: number
  profit?: number
  token_name?: string
  type?: number
}

export interface GrossProfitRow {
  key: string
  requests: number
  charge_quota: number
  cost_quota: number
  profit_quota: number
}

export interface LedgerRow {
  id: number
  kind: string
  quota_delta: number
  customer_id: number
  customer_username?: string
  money_usd?: number | null
  remark?: string
  created_at: number
}
