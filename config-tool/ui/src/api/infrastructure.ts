/** GET /api/infrastructure: Stage 4's dependencies, their modes and the questions each mode asks (ADR 0026). */
import { api } from './client'
import type { FoundationOption, FoundationType, TargetRef } from './foundations'

/** The three ways a dependency is provided, `model.ModeBundled`/`ModeCluster`/`ModeExternal` in Go. */
export const ModeId = { Bundled: 'bundled', Cluster: 'cluster', External: 'external' } as const
export type ModeId = (typeof ModeId)[keyof typeof ModeId]

/** The ingress a per-ingress question was expanded for; its row renders only while `owner` (a foundation id) resolves. */
export interface QuestionIngress {
  id: string
  label: string
  owner: string
}

/** Asks a question only while another question of the same mode resolves to `equals`. */
export interface QuestionWhen {
  question: string
  equals: string
}

/** One question of a mode, mirroring the server's `questionBody`; `id` is local to the dependency. */
export interface InfraQuestion {
  id: string
  type: FoundationType
  label: string
  help: string
  doc: string
  /** The values a choice offers; empty for every other type. */
  options: FoundationOption[]
  /** Template the answer is derived from, in Stage 3's syntax; empty when there is none. */
  default: string
  when: QuestionWhen | null
  readOnly: boolean
  /** Null for a plain question. */
  ingress: QuestionIngress | null
}

/** One mode of a dependency; `keys` are the chart keys it may write. Every string is bundle data. */
export interface Mode {
  id: ModeId
  label: string
  note: string
  warning: string
  doc: string
  development: boolean
  productionDoc: string
  keys: TargetRef[]
  questions: InfraQuestion[]
}

/** One Stage 4 dependency in bundle order, mirroring the server's `dependencyBody`. */
export interface Dependency {
  id: string
  title: string
  help: string
  doc: string
  clusterWide: boolean
  modes: Mode[]
}

/** Answer of GET /api/infrastructure. */
export interface InfrastructureList {
  dependencies: Dependency[]
}

/** GET /api/infrastructure; an empty `version` means the chart's latest. 404 for a chart or version without a bundle. */
export function fetchInfrastructure(chart: string, version: string, signal?: AbortSignal): Promise<InfrastructureList> {
  return api<InfrastructureList>(`/infrastructure?${new URLSearchParams({ chart, version })}`, { signal })
}
