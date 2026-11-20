/** Validation counters and per-stage status, computed purely from the store. */
import type { StageDef } from '../stages'
import type { AppState } from './reducer'
import { StageId } from './stageIds'

/** Stages 1–6, which the local trial skips: its values are bundled, not answered. */
export const TRIAL_SKIPPED: readonly StageId[] = [
  StageId.Target,
  StageId.Role,
  StageId.Foundations,
  StageId.Infrastructure,
  StageId.Features,
  StageId.Configuration,
]

/** Whether `stage` is skipped because the local trial is running. */
export function isSkipped(state: AppState, stage: StageId): boolean {
  return state.trial !== undefined && TRIAL_SKIPPED.includes(stage)
}

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

/** One stage's issues; zero when it has no selector or is skipped. */
export function stageIssues(state: AppState, stage: StageDef): Issues {
  return stage.selector && !isSkipped(state, stage.id) ? stage.selector(state) : NO_ISSUES
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
  Visited: 'visited',
  Skipped: 'skipped',
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
  visited: '',
  skipped: '–',
  none: '',
}

/** The MOCKUPS legend text of each status. */
export const STATUS_LEGEND: Record<StageStatus, string> = {
  current: 'current',
  error: 'has errors',
  warning: 'has warnings',
  open: 'has open questions',
  complete: 'complete',
  visited: 'visited, no status',
  skipped: 'skipped in the local trial',
  none: 'not visited',
}

/** A visited stage without issues is complete only if it has a selector; without one it is merely visited. */
export function stageStatus(state: AppState, stage: StageDef): StageStatus {
  if (stage.id === state.stage) return StageStatus.Current
  if (isSkipped(state, stage.id)) return StageStatus.Skipped
  const issues = stageIssues(state, stage)
  if (issues.errors > 0) return StageStatus.Error
  if (issues.warnings > 0) return StageStatus.Warning
  if (issues.open > 0) return StageStatus.Open
  if (!state.visited.includes(stage.id)) return StageStatus.None
  return stage.selector !== undefined ? StageStatus.Complete : StageStatus.Visited
}
