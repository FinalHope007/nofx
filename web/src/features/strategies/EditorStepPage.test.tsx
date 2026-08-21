import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { EditorStepPage } from './EditorStepPage'
import { LanguageProvider } from '../../contexts/LanguageContext'

vi.mock('../../lib/config', () => ({
  getSystemConfig: vi.fn().mockResolvedValue({ initialized: true }),
  invalidateSystemConfig: vi.fn(),
}))

vi.mock('../../lib/notify', () => ({
  notify: { error: vi.fn(), success: vi.fn() },
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ token: 'test-token' }),
}))

vi.mock('../../lib/api', () => ({
  api: {
    getStrategy: vi.fn(),
  },
}))

vi.mock('./strategyApi', () => ({
  strategyManagerApi: {
    getRunningTradersForStrategy: vi.fn().mockResolvedValue([]),
  },
}))

import { api } from '../../lib/api'
import { strategyManagerApi } from './strategyApi'
import { notify } from '../../lib/notify'

describe('EditorStepPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.setItem('auth_token', 'test-token')
    ;(api.getStrategy as ReturnType<typeof vi.fn>).mockResolvedValue({
      config: {
        ai_config: {
          decision_context: { enabled: true, recent_count: 3, mode: 'structured' },
          indicators: { klines: {} },
          coin_source: { source_type: 'binance_technical' },
          risk_control: {},
        },
      },
    } as any)
    ;(strategyManagerApi.getRunningTradersForStrategy as ReturnType<typeof vi.fn>).mockResolvedValue([])
  })

  const renderEditMode = () =>
    render(
      <MemoryRouter initialEntries={['/strategy/s1/edit']}>
        <LanguageProvider>
          <Routes>
            <Route path="/strategy/:id/edit" element={<EditorStepPage />} />
          </Routes>
        </LanguageProvider>
      </MemoryRouter>
    )

  it('renders Trading Style chips and a Risky throttle fieldset', () => {
    render(
      <MemoryRouter>
        <LanguageProvider>
          <EditorStepPage />
        </LanguageProvider>
      </MemoryRouter>
    )
    expect(screen.getByText('Trading Style')).toBeTruthy()
    expect(screen.getByText('Scalp')).toBeTruthy()
    expect(screen.getByText('Throttling Settings (Risky)')).toBeTruthy()
    expect(screen.getByLabelText(/min hold/i)).toBeTruthy()
  })

  it('renders Basic indicators and Data sources fieldsets', () => {
    render(
      <MemoryRouter>
        <LanguageProvider>
          <EditorStepPage />
        </LanguageProvider>
      </MemoryRouter>
    )
    expect(screen.getByText('Basic indicators')).toBeTruthy()
    expect(screen.getByText('Data sources')).toBeTruthy()
  })

  it('restores saved decision context count in edit mode', async () => {
    renderEditMode()
    // Wait for the async strategy load to complete and re-render.
    await waitFor(() => {
      expect(api.getStrategy).toHaveBeenCalledWith('s1')
    })
    // If the strategy load threw, notify.error would be called and the
    // decision-context restore (at the end of the try block) would be skipped.
    expect(notify.error).not.toHaveBeenCalled()
    await waitFor(() => {
      // The form body should render once the strategy load completes.
      expect(screen.getAllByText('Trading Style').length).toBeGreaterThan(0)
    })
    await waitFor(() => {
      expect(screen.getByText('Decisions in context')).toBeTruthy()
    })
    await waitFor(() => {
      const input = screen.getByRole('spinbutton', {
        name: /decisions in context/i,
      }) as HTMLInputElement
      expect(input.value).toBe('3')
    })
  })
})
