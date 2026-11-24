/** GET /api/foundations: the cross-cutting answers of a chart version and where each lands (ADR 0020). */
import { api } from './client'

/** Answer type, mirroring `model.FoundationType`; the server checks each with a pattern. */
export type FoundationType = 'string' | 'hostname' | 'domain'

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
