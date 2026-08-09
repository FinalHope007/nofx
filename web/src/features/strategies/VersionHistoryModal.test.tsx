import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { VersionHistoryModal } from './VersionHistoryModal'
import type { Strategy } from '../../types/strategy'

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
  it('shows current config version and a restore button', async () => {
    render(
      <VersionHistoryModal strategy={strategy} isOpen onClose={() => {}} />
    )
    await waitFor(() => {
      expect(screen.getByText(/v1/)).toBeInTheDocument()
      expect(screen.getByText(/Restore/)).toBeInTheDocument()
    })
  })
})
