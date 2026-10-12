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
import React, { useCallback, useMemo, useState } from 'react'

import useDialogState from '@/hooks/use-dialog'

import type { Reseller, ResellersDialogType } from '../types'

type Ctx = {
  open: ResellersDialogType
  setOpen: (v: ResellersDialogType) => void
  currentRow: Reseller | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Reseller | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const ResellersContext = React.createContext<Ctx | null>(null)

export function ResellersProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<NonNullable<ResellersDialogType>>(null)
  const [currentRow, setCurrentRow] = useState<Reseller | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const triggerRefresh = useCallback(
    () => setRefreshTrigger((n) => n + 1),
    []
  )
  const value = useMemo(
    () => ({
      open,
      setOpen,
      currentRow,
      setCurrentRow,
      refreshTrigger,
      triggerRefresh,
    }),
    [open, setOpen, currentRow, refreshTrigger, triggerRefresh]
  )
  return <ResellersContext value={value}>{children}</ResellersContext>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useResellers() {
  const ctx = React.useContext(ResellersContext)
  if (!ctx) throw new Error('useResellers must be used within ResellersProvider')
  return ctx
}
