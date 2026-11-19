/** Save, open and list `.dscprofile.yaml` profiles through the loopback API (ADR 0015). */
import { emptyAnswers, type Answers, type Target } from '../state/answers'
import { FIELD_IDS, type FieldId } from '../state/openQuestions'
import type { AppState } from '../state/reducer'
import { api } from './client'
import type { LoadReport } from './load'

/** Suffix every profile path must end in; the server refuses any other. */
export const PROFILE_SUFFIX = '.dscprofile.yaml'
/** How every profile path field explains what it takes. */
export const PATH_HINT = `An absolute path ending in ${PROFILE_SUFFIX}.`
/** Profile name when no release name is answered yet. */
export const DEFAULT_PROFILE_NAME = 'connector'
/** The server answers 409 when the path exists and `overwrite` is false. */
export const HTTP_CONFLICT = 409
/** `profile.FormatVersion`; the server sets it anyway. */
export const PROFILE_FORMAT = 1

/** The model bundle a profile was built with, mirroring `profile.ModelRef`. */
export interface ModelRef {
  chart: string
  version: string
  revision: number
}

/** The values file the overrides layer lives in; metadata only. */
export interface ProfileSource {
  file: string
  root: string
  sha256: string
}

/** The `.dscprofile.yaml` content, mirroring `profile.Profile` in Go. `answers` and `secrets` stay empty in CT-4. */
export interface Profile {
  format: number
  model: ModelRef
  target: Target
  answers: Record<string, unknown> | null
  open: { field: string }[] | null
  source?: ProfileSource
  secrets: Record<string, unknown> | null
}

/** Answer of save and open; `report` is set when open restored the source next to the profile. */
export interface ProfileResponse {
  path: string
  profile: Profile
  report?: LoadReport
  notes: string[]
}

/** One recent profile, newest first; `savedAt` is RFC 3339. */
export interface RecentEntry {
  path: string
  name: string
  chartVersion: string
  savedAt: string
  role?: string
}

/** Answer of GET /api/profile/recent; `defaultDir` is where a new profile goes by default. */
export interface RecentProfiles {
  entries: RecentEntry[]
  defaultDir: string
  notes: string[]
}

/** The profile the UI's state stands for; the server fills in format, model and secrets. */
export function toProfile(state: AppState): Profile {
  const target = state.answers.target
  return {
    format: PROFILE_FORMAT,
    model: state.profileModel ?? { chart: target.chart, version: '', revision: 0 },
    target,
    answers: {},
    open: state.open.map((field) => ({ field })),
    secrets: {},
  }
}

function isFieldId(id: string): id is FieldId {
  return (FIELD_IDS as readonly string[]).includes(id)
}

/** The answers and markers a profile restores; the server has validated its open ids already. */
export function fromProfile(profile: Profile): { answers: Answers; open: FieldId[] } {
  const answers = emptyAnswers()
  answers.target = { ...answers.target, ...profile.target }
  const open = (profile.open ?? []).map((q) => q.field).filter(isFieldId)
  return { answers, open: [...new Set(open)].sort() }
}

/** PUT /api/profile. The load the UI holds names the source; without one the profile references no values file.
 *  409 (exists, overwrite false) and 422 (bad path, invalid profile, stale load) arrive as ApiError. */
export function saveProfile(state: AppState, path: string, overwrite: boolean): Promise<ProfileResponse> {
  return api<ProfileResponse>('/profile', {
    method: 'PUT',
    body: JSON.stringify({ path, overwrite, profile: toProfile(state), sourceSha256: state.loaded?.sha256 }),
  })
}

/** POST /api/profile/open; 404 for a missing file, 422 for a bad path or an invalid profile. */
export function openProfile(path: string): Promise<ProfileResponse> {
  return api<ProfileResponse>('/profile/open', { method: 'POST', body: JSON.stringify({ path }) })
}

/** GET /api/profile/recent. */
export function fetchRecent(signal?: AbortSignal): Promise<RecentProfiles> {
  return api<RecentProfiles>('/profile/recent', { signal })
}

/** `dir` joined with `name`, using the separator `dir` already uses, so a Windows directory stays one. */
export function joinPath(dir: string, name: string): string {
  if (dir === '') return name
  const sep = dir.includes('\\') && !dir.includes('/') ? '\\' : '/'
  return dir.endsWith(sep) ? dir + name : dir + sep + name
}

/** Where a first save goes by default: `<dir>/<releaseName | connector>.dscprofile.yaml`. */
export function defaultProfilePath(dir: string, releaseName: string): string {
  return joinPath(dir, `${releaseName || DEFAULT_PROFILE_NAME}${PROFILE_SUFFIX}`)
}

/** The file name without directory and suffix, as the recent list and the title bar name a profile. */
export function profileName(path: string): string {
  const base = path.split(/[\\/]/).pop() ?? path
  return base.endsWith(PROFILE_SUFFIX) ? base.slice(0, -PROFILE_SUFFIX.length) : base
}

const MINUTE_MS = 60_000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS
const MONTH_MS = 30 * DAY_MS
const YEAR_MS = 365 * DAY_MS
const relativeTime = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

/** `savedAt` relative to `now`, in the largest unit that fits. */
export function sinceSaved(savedAt: string, now: number = Date.now()): string {
  const elapsed = Date.parse(savedAt) - now
  if (Number.isNaN(elapsed)) return savedAt
  if (Math.abs(elapsed) < HOUR_MS) return relativeTime.format(Math.round(elapsed / MINUTE_MS), 'minute')
  if (Math.abs(elapsed) < DAY_MS) return relativeTime.format(Math.round(elapsed / HOUR_MS), 'hour')
  if (Math.abs(elapsed) < MONTH_MS) return relativeTime.format(Math.round(elapsed / DAY_MS), 'day')
  if (Math.abs(elapsed) < YEAR_MS) return relativeTime.format(Math.round(elapsed / MONTH_MS), 'month')
  return relativeTime.format(Math.round(elapsed / YEAR_MS), 'year')
}
