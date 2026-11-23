import { describe, expect, it } from 'vitest'
import type { FeatureChoice } from '../api/features'
import { deliberateLabel } from './deliberate'

describe('deliberateLabel', () => {
  it.each<[Pick<FeatureChoice, 'offer' | 'enabled' | 'source'>, string]>([
    [{ offer: 'recommended', enabled: false, source: 'user' }, 'recommended on for Provider — you turned it off'],
    [{ offer: 'recommended', enabled: false, source: 'loaded' }, 'recommended on for Provider — your values file turns it off'],
    [{ offer: 'available', enabled: true, source: 'user' }, 'off by default for Provider — you turned it on'],
    [{ offer: 'available', enabled: true, source: 'loaded' }, 'off by default for Provider — your values file turns it on'],
    [{ offer: 'not-offered', enabled: true, source: 'user' }, 'not recommended for Provider — you turned it on'],
    [{ offer: 'not-offered', enabled: true, source: 'loaded' }, 'not recommended for Provider — your values file turns it on'],
  ])('words %o', (choice, label) => {
    expect(deliberateLabel(choice, 'Provider')).toBe(label)
  })
})
