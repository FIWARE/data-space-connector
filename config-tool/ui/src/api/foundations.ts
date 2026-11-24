/** GET /api/foundations: the cross-cutting answers of a chart version and where each lands (ADR 0020). */
import { api } from './client'

/** Answer type, mirroring `model.FoundationType`; a choice is checked against its options, every other type with a pattern. */
export type FoundationType = 'string' | 'hostname' | 'domain' | 'choice' | 'did' | 'url' | 'bool' | 'credentialTypes'

/** One element of a `credentialTypes` answer, mirroring the Go check; types are unique within the list. */
export interface CredentialType {
  type: string
  description?: string
}

/** One value a choice offers, mirroring `model.Option`; `note` is shown at the point of choice. */
export interface FoundationOption {
  value: string
  label: string
  note: string
}

/** The Stage 3 panel that asks a foundation, mirroring `model.FoundationGroup`. */
export type FoundationGroup = 'organization' | 'dataspace' | 'network'

/** Holds while the resolved answer `id` equals `equals`, mirroring `model.Condition`. */
export interface Condition {
  id: string
  equals: string
}

/** One chart key a foundation reaches; `component` is the path's top-level key. */
export interface TargetRef {
  path: string
  component: string
}

/** One foundation in bundle order, mirroring the server's `foundationBody`. */
export interface Foundation {
  id: string
  stage: string
  type: FoundationType
  group: FoundationGroup
  /** Template the answer is derived from; empty for an answer the user gives. */
  default: string
  /** Gates `default` on another answer; null when the default always applies. */
  defaultWhen: Condition | null
  /** Template the answer, given or derived, must start with; empty when there is none. */
  prefix: string
  /** The values a choice offers; empty for every other type. */
  options: FoundationOption[]
  label: string
  help: string
  /** Repo-relative doc path, empty when there is none. */
  doc: string
  /** Every key the answer reaches, its derived answers' keys included; sorted by path. */
  targets: TargetRef[]
  count: number
  components: string[]
  /** Feature ids; Stage 3 hides the foundation while any is off. Empty when it is always shown. */
  features: string[]
}

/** Answer of GET /api/foundations. */
export interface FoundationList {
  foundations: Foundation[]
}

/** GET /api/foundations; an empty `version` means the chart's latest. 404 for a chart or version without a bundle. */
export function fetchFoundations(chart: string, version: string, signal?: AbortSignal): Promise<FoundationList> {
  return api<FoundationList>(`/foundations?${new URLSearchParams({ chart, version })}`, { signal })
}
