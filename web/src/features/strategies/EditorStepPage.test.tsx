import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { EditorStepPage } from './EditorStepPage'
import { AuthProvider } from '../../contexts/AuthContext'
import { LanguageProvider } from '../../contexts/LanguageContext'

vi.mock('../../lib/config', () => ({
  getSystemConfig: vi.fn().mockResolvedValue({ initialized: true }),
  invalidateSystemConfig: vi.fn(),
}))

describe('EditorStepPage', () => {
  it('renders Trading Style chips and a Risky throttle fieldset', () => {
    render(
      <MemoryRouter>
        <LanguageProvider>
          <AuthProvider>
            <EditorStepPage />
          </AuthProvider>
        </LanguageProvider>
      </MemoryRouter>
    )
    expect(screen.getByText('Trading Style')).toBeTruthy()
    expect(screen.getByText('Scalp')).toBeTruthy()
    expect(screen.getByText('Throttling Settings (Risky)')).toBeTruthy()
    expect(screen.getByLabelText(/min hold/i)).toBeTruthy()
  })
})
