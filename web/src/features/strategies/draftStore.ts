import { create } from 'zustand'
import type { ScopeUnit } from '../../types/strategy'

interface StrategyDraft {
  scope: {
    units: ScopeUnit[]
    mode: 'overlap' | 'union'
  }
  setScopeUnits: (units: ScopeUnit[]) => void
  setScopeMode: (mode: 'overlap' | 'union') => void
  mergeScopeUnit: (unit: ScopeUnit) => void
  removeScopeUnit: (id: string) => void
  resetDraft: () => void
}

export const useStrategyDraft = create<StrategyDraft>((set) => ({
  scope: { units: [], mode: 'union' },
  setScopeUnits: (units) => set({ scope: { mode: 'union', units } }),
  setScopeMode: (mode) => set((s) => ({ scope: { ...s.scope, mode } })),
  mergeScopeUnit: (unit) =>
    set((s) => {
      const exists = s.scope.units.some((u) => u.id === unit.id)
      const units = exists
        ? s.scope.units.map((u) => (u.id === unit.id ? unit : u))
        : [...s.scope.units, unit]
      return { scope: { ...s.scope, units } }
    }),
  removeScopeUnit: (id) =>
    set((s) => ({
      scope: { ...s.scope, units: s.scope.units.filter((u) => u.id !== id) },
    })),
  resetDraft: () => set({ scope: { units: [], mode: 'union' } }),
}))

export function resetStrategyDraft() {
  useStrategyDraft.getState().resetDraft()
}
