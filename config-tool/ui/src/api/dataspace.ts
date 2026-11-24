/** POST /api/dataspace/check and /api/dataspace/import: the only outbound calls, each sent on a button press (ADR 0022). */
import type { AnswerValue } from '../state/answers'
import { api } from './client'
import type { Profile } from './profile'

/** What the TIR check found, mirroring `server.reachStatus*`; any HTTP answer counts as reachable. */
export type Reachability = 'reachable' | 'unreachable'

/** Answer of POST /api/dataspace/check; `httpStatus` comes with reachable, `error` with unreachable. */
export interface TIRCheckResult {
  status: Reachability
  httpStatus?: number
  error?: string
}

/** An imported value that differs from the given answer, mirroring `dataspace.Change`; `current` is absent when unanswered. */
export interface ImportChange {
  id: string
  current?: AnswerValue
  imported: AnswerValue
}

/** An imported value its answer's type refuses, mirroring `dataspace.Problem`; it is not offered as a change. */
export interface ImportProblem {
  id: string
  /** The descriptor key the value came from. */
  key: string
  message: string
}

/** What applying a descriptor would change, mirroring `dataspace.PreviewResult`; nothing is applied by the server. */
export interface ImportPreview {
  changes: ImportChange[]
  problems: ImportProblem[]
  /** The chart's own descriptor keys; shown, never applied. */
  known?: Record<string, unknown>
  unused: string[]
  /** Set when the descriptor publishes none of the imported keys; information, not a failure. */
  message?: string
}

/** Answer of POST /api/dataspace/import; `url` is the descriptor address actually fetched. */
export interface ImportResult {
  url: string
  preview: ImportPreview
}

/** POST /api/dataspace/check; 422 (malformed URL) arrives as ApiError, an unreachable host as `status: unreachable`. */
export function checkTIR(url: string, signal?: AbortSignal): Promise<TIRCheckResult> {
  return api<TIRCheckResult>('/dataspace/check', { method: 'POST', body: JSON.stringify({ url }), signal })
}

/** POST /api/dataspace/import; 422 (bad URL or profile) and 502 (fetch or parse failed) arrive as ApiError. */
export function importDescriptor(url: string, profile: Profile, signal?: AbortSignal): Promise<ImportResult> {
  return api<ImportResult>('/dataspace/import', { method: 'POST', body: JSON.stringify({ url, profile }), signal })
}
