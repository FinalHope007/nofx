import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { VersionHistoryModal } from './VersionHistoryModal'
import type { Strategy, StrategyVersion } from '../../types/strategy'

vi.mock('../../lib/api/strategies', () => ({
  strategyApi: {
    getVersions: vi.fn().mockResolvedValue([
      {
        version: 1,
        strategy_id: 's1',
        label: 'v1',
        note: 'Current configuration',
        config: { strategy_type: 'ai_trading', language: 'en' },
        created_at: '2026-01-01T00:00:00Z',
        is_current: true,
      } as StrategyVersion,
    ]),
  },
}))

const strategy: Strategy = {
  id: 's1',
  name: 'Test strategy',
  description: '',
  is_active: true,
  is_default: false,
  is_public: false,
  config_visible: true,
  config: { strategy_type: 'ai_trading', language: 'en' },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

describe('VersionHistoryModal', () => {
  it('shows a version option and a restore button', async () => {
    render(
      <VersionHistoryModal strategy={strategy} isOpen onClose={() => {}} />
    )
    await waitFor(() => {
      expect(screen.getByText(/v1/)).toBeInTheDocument()
      expect(screen.getByText(/Restore/)).toBeInTheDocument()
    })
  })
})
