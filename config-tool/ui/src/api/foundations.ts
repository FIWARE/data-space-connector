/** GET /api/foundations: the cross-cutting answers of a chart version and where each lands (ADR 0020). */
import { api } from './client'

/** Answer type, mirroring `model.FoundationType`; a choice is checked against its options, every other type with a pattern. */
export type FoundationType = 'string' | 'hostname' | 'domain' | 'choice' | 'did'

/** One value a choice offers, mirroring `model.Option`; `note` is shown at the point of choice. */
export interface FoundationOption {
  value: string
  label: string
  note: string
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
  /** Template the answer is derived from; empty for an answer the user gives. */
  default: string
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
}

/** Answer of GET /api/foundations. */
export interface FoundationList {
  foundations: Foundation[]
}

/** GET /api/foundations; an empty `version` means the chart's latest. 404 for a chart or version without a bundle. */
export function fetchFoundations(chart: string, version: string, signal?: AbortSignal): Promise<FoundationList> {
  return api<FoundationList>(`/foundations?${new URLSearchParams({ chart, version })}`, { signal })
}
