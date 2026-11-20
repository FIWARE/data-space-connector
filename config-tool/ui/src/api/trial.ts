/** GET /api/trial and POST /api/trial/write: the bundled local trial (ADR 0016). */
import { api } from './client'

/** One release of the trial, in install order; `values` is the full rendered file, header and banner included. */
export interface TrialRelease {
  role: string
  releaseName: string
  namespace: string
  chart: string
  chartVersion: string
  /** The `k3s/…` file the bundled values are byte-identical to. */
  source: string
  /** Output file name inside the chosen directory. */
  file: string
  values: string
}

/** A production warning; always shown, never dismissed. `releases` names the release names it applies to. */
export interface TrialWarning {
  id: string
  releases: string[]
  message: string
  link?: string
}

/** Body of GET /api/trial, mirroring `server.trialBody`. */
export interface Trial {
  profile: string
  environment: string
  releases: TrialRelease[]
  warnings: TrialWarning[]
}

/** Answer of POST /api/trial/write; `files` are the absolute paths written, in install order. */
export interface TrialWritten {
  dir: string
  files: string[]
}

/** GET /api/trial. */
export function fetchTrial(signal?: AbortSignal): Promise<Trial> {
  return api<Trial>('/trial', { signal })
}

/** POST /api/trial/write; writes all three files or none. 409 (a target exists) and 422 (bad dir) arrive as ApiError. */
export function writeTrial(dir: string): Promise<TrialWritten> {
  return api<TrialWritten>('/trial/write', { method: 'POST', body: JSON.stringify({ dir }) })
}
