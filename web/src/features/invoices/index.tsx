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
import { requireServerSuccess } from '@/lib/server-error-message'

import { adminListResellerLedger } from './api'
import { BalanceLedgerPage } from './components/balance-ledger-page'
import {
  RESELLER_LEDGER_KINDS,
  type LedgerPage,
  type LedgerQuery,
  type ResellerLedgerRow,
} from './types'

async function loadAdminLedger(query: LedgerQuery): Promise<LedgerPage> {
  const res = requireServerSuccess(
    await adminListResellerLedger({
      period: query.period,
      kind: query.kind || undefined,
      p: query.page,
      page_size: query.pageSize,
    })
  )
  return {
    items: (res.data?.items ?? []) as ResellerLedgerRow[],
    total: res.data?.total ?? 0,
  }
}

export function AdminInvoicesPage() {
  return (
    <BalanceLedgerPage
      queryKey='admin-reseller-ledger-range'
      loadRows={loadAdminLedger}
      exportUrl='/api/admin/reseller_ledger/export'
      kinds={RESELLER_LEDGER_KINDS}
      showReseller
      showCustomer
    />
  )
}
