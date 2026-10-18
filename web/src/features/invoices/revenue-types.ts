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
export type RevenueScope = 'admin' | 'reseller'

export type RevenueAgg = {
  key: string
  id: number
  name: string
  requests: number
  refunds: number
  customer_price: number
  reseller_cost: number
  reseller_profit: number
  admin_revenue: number
  base_quota_micro?: number
  admin_profit_micro?: number
  flagged_rows: number
}

export type RevenueReconciliation = {
  identity_ok: boolean
  identity_diff_micro: number
  sections_ok: boolean
  log_compared: boolean
  log_charge_quota: number
  log_reseller_cost: number
  log_charge_diff: number
  log_reseller_cost_diff: number
  flagged_rows: number
  pending_retry: number
}

export type RevenueStatement = {
  period: string
  timezone: string
  scope: RevenueScope
  reseller_id?: number
  reseller_name?: string
  totals: RevenueAgg
  customer_total: RevenueAgg
  self_total: RevenueAgg
  direct_total: RevenueAgg
  by_reseller?: RevenueAgg[]
  reseller_self?: RevenueAgg[]
  direct?: RevenueAgg[]
  by_customer?: RevenueAgg[]
  reconciliation: RevenueReconciliation
  row_count: number
  checksum: string
  frozen: boolean
  frozen_at?: number
  late_rows: number
  checksum_mismatch: boolean
  generated_at: number
}
