import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('EditorStepPage AltFins controls', () => {
  const src = readFileSync(resolve(__dirname, 'EditorStepPage.tsx'), 'utf8')
  it('renders AltFins toggle and interval multiselect', () => {
    expect(src).toContain('AltFins')
    expect(src).toContain('setAltfinsIntervals')
    expect(src).toContain("'MINUTES15'")
  })
  it('renders vergex per-coin toggles', () => {
    expect(src).toContain('Vergex Signal Lab')
    expect(src).toContain('Vergex Liquidation Heatmap')
  })
  it('hides vergex per-coin toggles for vergex scopes', () => {
    expect(src).toContain("const isVergexScope = scope?.source_type === 'vergex'")
    expect(src).toContain('isVergexScope ?')
  })
})
