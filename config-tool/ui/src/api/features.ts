/** POST /api/features: the user's toggles over the role preset and how each feature stands (ADR 0018). */
import { api } from './client'
import type { FeatureSource, Profile } from './profile'
import type { Offer } from './roles'

export type { FeatureSource } from './profile'

/** One feature of the target chart next to its preset value, mirroring `profile.FeatureChoice`; the UI words it. */
export interface FeatureChoice {
  id: string
  offer: Offer
  preset: boolean
  enabled: boolean
  /** The state differs from the preset and the preset did not decide it. */
  deliberate: boolean
  source: FeatureSource
}

/** A feature the user switches on or off. */
export interface FeatureToggle {
  feature: string
  enabled: boolean
}

/** Answer of POST /api/features; `choices` are in bundle order and cover the target chart only. */
export interface FeaturesResult {
  profile: Profile
  choices: FeatureChoice[]
}

/** POST /api/features, recording `toggle` as source user when given. Only call it once a role is picked: 422 (no role,
 *  unknown feature, a feature of another chart, invalid profile, stale load) arrives as ApiError. */
export function fetchFeatures(profile: Profile, sourceSha256?: string, toggle?: FeatureToggle): Promise<FeaturesResult> {
  return api<FeaturesResult>('/features', { method: 'POST', body: JSON.stringify({ profile, sourceSha256, toggle: toggle ?? null }) })
}
