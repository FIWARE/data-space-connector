/** Wording of a deliberate choice, shared by S5 Features and S7 Review so the two cannot drift. */
import type { FeatureChoice } from '../api/features'
import type { Offer } from '../api/roles'

const PRESET_WORDS: Record<Offer, string> = {
  recommended: 'recommended on for',
  available: 'off by default for',
  'not-offered': 'not recommended for',
}

/** Why a feature differs from the preset: what the role wants, then who decided otherwise. */
export function deliberateLabel(choice: Pick<FeatureChoice, 'offer' | 'enabled' | 'source'>, roleTitle: string): string {
  const state = choice.enabled ? 'on' : 'off'
  const who = choice.source === 'loaded' ? `your values file turns it ${state}` : `you turned it ${state}`
  return `${PRESET_WORDS[choice.offer]} ${roleTitle} — ${who}`
}
