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
import { BalanceLedgerPage } from '@/features/invoices/components/balance-ledger-page'
import { InvoicesTabsPage } from '@/features/invoices/components/invoices-tabs'
import {
  RESELLER_LEDGER_KINDS,
  type LedgerPage,
  type LedgerQuery,
  type ResellerLedgerRow,
} from '@/features/invoices/types'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getLedger } from '../api'

async function loadResellerLedger(query: LedgerQuery): Promise<LedgerPage> {
  const res = requireServerSuccess(
    await getLedger({
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

export function ResellerInvoicesPage() {
  return (
    <InvoicesTabsPage
      scope='reseller'
      renderLedger={(tabSwitch) => (
        <BalanceLedgerPage
          queryKey='reseller-ledger-range'
          loadRows={loadResellerLedger}
          exportUrl='/api/reseller/ledger/export'
          kinds={RESELLER_LEDGER_KINDS}
          showCustomer
          extraActions={tabSwitch}
        />
      )}
    />
  )
}
