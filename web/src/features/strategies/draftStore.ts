import { create } from 'zustand'
import type { ScopeUnit } from '../../types/strategy'

interface StrategyDraft {
  scope: ScopeUnit | null
  setScope: (unit: ScopeUnit) => void
  clearScope: () => void
  resetDraft: () => void
}

export const useStrategyDraft = create<StrategyDraft>((set) => ({
  scope: null,
  setScope: (unit) => set({ scope: unit }),
  clearScope: () => set({ scope: null }),
  resetDraft: () => set({ scope: null }),
}))

export function resetStrategyDraft() {
  useStrategyDraft.getState().resetDraft()
}
