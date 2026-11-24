/** POST /api/render: the values tree the answers render to and its change since the last render (ADR 0020). */
import { api } from './client'
import type { Profile } from './profile'

/** Where a leaf came from, mirroring `render.Provenance`. */
export interface Provenance {
  foundation: string
  answers: string[]
  fn: string
  target: number
  /** A hand edit of the loaded file the render keeps; `value` is then the kept scalar. */
  pinned: boolean
  /** A pinned leaf has a kept scalar; false when the loaded node is not one. */
  kept: boolean
  /** What the render would have written; set on a pinned leaf only. */
  rendered?: unknown
}

/** One scalar of the tree; `path` is in roundtrip.ParsePath syntax. */
export interface Leaf {
  path: string
  value: unknown
  provenance: Provenance
}

/** How a leaf changed between two renders. */
export type ChangeStatus = 'added' | 'changed' | 'removed' | 'pinned'

/** One changed leaf; a pinned one carries the two rendered values and, when it has one, the kept value. */
export interface Change {
  path: string
  status: ChangeStatus
  before?: unknown
  after?: unknown
  kept?: unknown
}

/** One foundation's changes, sorted by path. */
export interface ChangeGroup {
  foundation: string
  changes: Change[]
}

/** A given answer that overrides a default derived from other answers, mirroring `render.HandSetAnswer`. */
export interface HandSetAnswer {
  id: string
  value: unknown
  /** What the default gives now; empty when it would derive nothing. */
  derived: string
}

/** Answer of POST /api/render; the groups are in bundle order and the first render diffs against an empty tree. */
export interface RenderResult {
  tree: { leaves: Leaf[] }
  diff: { groups: ChangeGroup[] }
  handSet: HandSetAnswer[]
}

/** POST /api/render. `sourceSha256` names the held load as on save; without it nothing is pinned.
 *  422 (invalid profile, stale load) arrives as ApiError. */
export function fetchRender(profile: Profile, sourceSha256?: string, signal?: AbortSignal): Promise<RenderResult> {
  return api<RenderResult>('/render', { method: 'POST', body: JSON.stringify({ profile, sourceSha256 }), signal })
}
