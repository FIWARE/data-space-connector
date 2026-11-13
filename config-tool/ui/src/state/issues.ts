/** Validation counters and per-stage status, computed purely from the store. */
import type { StageDef } from '../stages'
import type { AppState } from './reducer'

/** What the footer counts and the rail marks. */
export interface Issues {
  warnings: number
  errors: number
  open: number
}

/** Derives a stage's issues from the store. */
export type IssueSelector = (state: AppState) => Issues

/** The counters of a stage without a selector. */
export const NO_ISSUES: Issues = { warnings: 0, errors: 0, open: 0 }

/** One stage's issues; zero when it has no selector. */
export function stageIssues(state: AppState, stage: StageDef): Issues {
  return stage.selector ? stage.selector(state) : NO_ISSUES
}

/** The footer totals over all stages. */
export function countIssues(state: AppState, stages: readonly StageDef[]): Issues {
  return stages.reduce<Issues>((sum, stage) => {
    const issues = stageIssues(state, stage)
    return { warnings: sum.warnings + issues.warnings, errors: sum.errors + issues.errors, open: sum.open + issues.open }
  }, NO_ISSUES)
}

/** Rail status of a stage, worst first after "current". */
export const StageStatus = {
  Current: 'current',
  Error: 'error',
  Warning: 'warning',
  Open: 'open',
  Complete: 'complete',
  None: 'none',
} as const
export type StageStatus = (typeof StageStatus)[keyof typeof StageStatus]

/** The rail glyph of each status, as in the MOCKUPS legend. */
export const STATUS_GLYPH: Record<StageStatus, string> = {
  current: '●',
  error: '✖',
  warning: '⚠',
  open: '○',
  complete: '✓',
  none: '',
}

/** The MOCKUPS legend text of each status. */
export const STATUS_LEGEND: Record<StageStatus, string> = {
  current: 'current',
  error: 'has errors',
  warning: 'has warnings',
  open: 'has open questions',
  complete: 'complete',
  none: 'not visited',
}

/** A visited stage without issues is complete only if it has a selector; otherwise there is nothing to vouch for it. */
export function stageStatus(state: AppState, stage: StageDef): StageStatus {
  if (stage.id === state.stage) return StageStatus.Current
  const issues = stageIssues(state, stage)
  if (issues.errors > 0) return StageStatus.Error
  if (issues.warnings > 0) return StageStatus.Warning
  if (issues.open > 0) return StageStatus.Open
  if (stage.selector !== undefined && state.visited.includes(stage.id)) return StageStatus.Complete
  return StageStatus.None
}
