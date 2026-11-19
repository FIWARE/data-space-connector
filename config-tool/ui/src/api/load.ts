/** POST /api/load: uploads an existing values.yaml and returns its load report (ADR 0014). */
import { api } from './client'

/** Content type of the upload; set explicitly because the client defaults a body to JSON. */
export const CONTENT_TYPE_YAML = 'application/yaml'
/** Query parameter carrying the file's name. */
export const QUERY_NAME = 'name'

/** Where the report's chart version comes from. */
export const VersionSource = { Header: 'header', Latest: 'latest' } as const
export type VersionSource = (typeof VersionSource)[keyof typeof VersionSource]

/** Which class of leaf set an answer; an owned leaf always wins. */
export type Origin = 'owned' | 'recognized'

/** One leaf or skipped pair; `line` is the file line, `value` a display preview. */
export interface LoadEntry {
  path: string
  line: number
  value: string
  answer?: string
  note?: string
}

/** Lengths of the four lists; `total` is their sum. */
export interface LoadCounts {
  owned: number
  recognized: number
  unknown: number
  skipped: number
  total: number
}

/** A back-derived answer; `value` is a bool for toggles. `stage` is a StageId value. */
export interface DerivedAnswer {
  id: string
  stage: string
  value: boolean | string | number
  paths: string[]
  from: Origin
}

/** An owned marker; `underived` lists the answer ids no owned leaf derived. */
export interface OwnedMarker {
  path: string
  line: number
  answerIds: string[]
  underived: string[]
}

/** Body of POST /api/load, mirroring `load.Report` in Go. */
export interface LoadReport {
  source: string
  chart: string
  chartVersion: string
  chartVersionFrom: VersionSource
  chartVersionNote?: string
  generated: boolean
  counts: LoadCounts
  owned: LoadEntry[]
  recognized: LoadEntry[]
  unknown: LoadEntry[]
  skipped: LoadEntry[]
  answers: DerivedAnswer[]
  markers: OwnedMarker[]
}

/** Uploads `file` as is, never decoded, so a BOM or invalid UTF-8 reaches the server untouched.
 *  413 (too large) and 422 (not one YAML document) arrive as ApiError. */
export function loadValues(file: File, signal?: AbortSignal): Promise<LoadReport> {
  return api<LoadReport>(`/load?${QUERY_NAME}=${encodeURIComponent(file.name)}`, {
    method: 'POST',
    headers: { 'Content-Type': CONTENT_TYPE_YAML },
    body: file,
    signal,
  })
}
