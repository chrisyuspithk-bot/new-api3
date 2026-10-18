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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { RevenueScope } from '../revenue-types'
import { RevenueStatementPage } from './revenue-statement-page'

type InvoiceTab = 'ledger' | 'revenue'

/**
 * Invoices page with two tabs: the existing balance ledger and the revenue
 * (earnings) statement. The tab switch renders in each page's action bar.
 */
export function InvoicesTabsPage(props: {
  scope: RevenueScope
  renderLedger: (tabSwitch: ReactNode) => ReactNode
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<InvoiceTab>('ledger')
  const tabSwitch = (
    <Tabs value={tab} onValueChange={(value) => setTab(value as InvoiceTab)}>
      <TabsList>
        <TabsTrigger value='ledger'>{t('Balance ledger')}</TabsTrigger>
        <TabsTrigger value='revenue'>{t('Revenue')}</TabsTrigger>
      </TabsList>
    </Tabs>
  )
  if (tab === 'revenue') {
    return <RevenueStatementPage scope={props.scope} tabSwitch={tabSwitch} />
  }
  return <>{props.renderLedger(tabSwitch)}</>
}
