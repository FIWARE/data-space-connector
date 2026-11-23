/** GET /api/roles and POST /api/preset: the role presets of the model bundle (ADR 0017). */
import { api } from './client'
import type { FeatureSource, Profile } from './profile'

/** How a role preset offers a feature. */
export type Offer = 'recommended' | 'available' | 'not-offered'

/** A role's minimum footprint as Kubernetes quantities, measured with chart version `measuredWith`. */
export interface Floor {
  memory: string
  cpu: string
  measuredWith: string
}

/** One role preset; `floor` is null when the footprint depends on the selection (custom). */
export interface Role {
  id: string
  title: string
  summary: string
  chart: string
  floor: Floor | null
  docs: string
  features: Record<string, Offer>
}

/** One entry of the feature catalogue. */
export interface Feature {
  id: string
  title: string
  summary: string
  group: string
  /** What turning the feature off costs; empty when the bundle names none. */
  consequence: string
}

/** Body of GET /api/roles, mirroring `server.rolesResponse`; both lists are in bundle order. */
export interface RoleCatalogue {
  roles: Role[]
  features: Feature[]
}

/** The target chart and version a preset replaced. */
export interface ChartSwitch {
  fromChart: string
  fromVersion: string
  toChart: string
  /** User toggles of the old chart that the new chart cannot carry. */
  droppedChoices: string[]
  /** Answer ids of the target and foundations stages, which survive the switch; sorted. */
  carriedAnswers: string[]
  /** Every other answer id; sorted. */
  droppedAnswers: string[]
}

/** A user or values-file toggle a role change keeps in its state. */
export interface KeptFeature {
  id: string
  enabled: boolean
  source: FeatureSource
}

/** The summary of a role change, mirroring `profile.RoleChange` (ADR 0019): ids only, in catalogue order. */
export interface RoleChange {
  fromRole: string
  toRole: string
  /** Features whose effective state goes off -> on, and on -> off. */
  enabled: string[]
  disabled: string[]
  kept: KeptFeature[]
}

/** Answer of POST /api/preset; `chartSwitch` is null when the role deploys the profile's chart, `change` on the
 *  first pick (the profile had no role). */
export interface PresetResult {
  profile: Profile
  chartSwitch: ChartSwitch | null
  change: RoleChange | null
}

/** GET /api/roles; an empty `version` means the chart's latest. 404 for a chart or version without a bundle. */
export function fetchRoles(chart: string, version: string, signal?: AbortSignal): Promise<RoleCatalogue> {
  return api<RoleCatalogue>(`/roles?${new URLSearchParams({ chart, version })}`, { signal })
}

/** POST /api/preset. `sourceSha256` names the held load, as on save; 422 (unknown role, invalid profile, a chart
 *  switch while a file is loaded, stale load) arrives as ApiError. */
export function applyPreset(profile: Profile, role: string, sourceSha256?: string): Promise<PresetResult> {
  return api<PresetResult>('/preset', { method: 'POST', body: JSON.stringify({ profile, role, sourceSha256 }) })
}
