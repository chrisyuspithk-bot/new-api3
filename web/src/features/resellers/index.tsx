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
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'

import { ResellersMutateDrawer } from './components/resellers-mutate-drawer'
import { ResellersPrimaryButtons } from './components/resellers-primary-buttons'
import { ResellersProvider, useResellers } from './components/resellers-provider'
import { ResellersQuotaDialog } from './components/resellers-quota-dialog'
import { ResellersTable } from './components/resellers-table'
import { ResellerReportsPanel } from './components/reseller-reports-panel'

function ResellersContent() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useResellers()

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Resellers')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ResellersPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-6'>
            <ResellersTable />
            <ResellerReportsPanel />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ResellersMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? (currentRow ?? undefined) : undefined}
      />
      <ResellersQuotaDialog
        open={open === 'credit' || open === 'debit'}
        mode={open === 'debit' ? 'debit' : 'credit'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={currentRow}
      />
    </>
  )
}

export function Resellers() {
  return (
    <ResellersProvider>
      <ResellersContent />
    </ResellersProvider>
  )
}
