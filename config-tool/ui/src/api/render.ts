/** POST /api/render: the values tree the answers render to and its change since the last render (ADR 0020). */
import { api } from './client'
import type { StageId } from '../state/stageIds'
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
  /** The key of the list element the leaf lies in, for a list written by keyed elements. */
  element?: string
  /** `base` or `loaded` on a leaf of an element the render keeps but does not own. */
  origin?: 'base' | 'loaded'
}

/** One scalar of the tree; `path` is in roundtrip.ParsePath syntax. */
export interface Leaf {
  path: string
  value: unknown
  provenance: Provenance
}

/** How a leaf changed between two renders. */
export type ChangeStatus = 'added' | 'changed' | 'removed' | 'pinned'

/** One changed leaf, or a pinned keyed element (`list[field=key]`, values whole elements); a pinned one carries the two rendered values and, when it has one, the kept value. */
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

/** An unanswered derived answer whose resolved value changed, mirroring `render.DerivedChange`; null is unresolved. */
export interface DerivedChange {
  id: string
  before: unknown
  after: unknown
}

/** A given answer kept while its derived value moved: *pinned, not updated*. `before`/`after` are the derived values,
 *  empty when nothing derives. */
export interface PinnedAnswer {
  id: string
  kept: unknown
  before: string
  after: string
}

/** What the answer change since the previous render did to the derived answers, mirroring `render.AnswerDiff`.
 *  Both lists are in foundation order and empty on the first render after a load, open or chart/version switch. */
export interface AnswerDiff {
  rederived: DerivedChange[]
  pinned: PinnedAnswer[]
}

/** How much an L2 finding weighs; only an error blocks generate (ADR 0021). */
export type Severity = 'error' | 'warning'

/** One bundle rule that holds for the profile, mirroring `check.Finding`; `field` is the open-field id it points at. */
export interface Finding {
  rule: string
  severity: Severity
  stage: StageId
  field: string
  /** The other field of a host-collision or did-web-host finding. */
  related?: string
  message: string
  /** A repository-relative doc path, optionally with an anchor. */
  doc: string
}

/** A mechanism none of whose targets rendered, mirroring `render.Withheld` (ADR 0025); `field`, `value` and
 *  `foreignKey` are set for a collision only, `conflict` for a conflict only. */
export interface Withheld {
  reason: 'collision' | 'edited' | 'conflict'
  mechanism: string
  element: string
  list: string
  field?: string
  value?: unknown
  foreignKey?: string
  conflict?: string[]
}

/** A Secret the values reference but the tool never creates, mirroring `requirements.Secret`. */
export interface RequiredSecret {
  /** Empty while Stage 1 leaves the namespace unanswered. */
  namespace?: string
  name: string
  key: string
  purpose: string
}

/** Answer of POST /api/render; the groups are in bundle order and the first render diffs against an empty tree.
 *  Findings come with every render, also an unchanged one, and never fail it. */
export interface RenderResult {
  tree: { leaves: Leaf[]; withheld?: Withheld[] }
  diff: { groups: ChangeGroup[] }
  derived: AnswerDiff
  handSet: HandSetAnswer[]
  findings: Finding[]
  secrets: RequiredSecret[]
}

/** POST /api/render. `sourceSha256` names the held load as on save; without it nothing is pinned.
 *  422 (invalid profile, stale load) arrives as ApiError. */
export function fetchRender(profile: Profile, sourceSha256?: string, signal?: AbortSignal): Promise<RenderResult> {
  return api<RenderResult>('/render', { method: 'POST', body: JSON.stringify({ profile, sourceSha256 }), signal })
}
