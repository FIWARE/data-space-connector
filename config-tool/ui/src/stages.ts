/** The stage rail: every stage of the wizard in order, and what renders it. */
import type { ComponentType } from 'react'
import type { IssueSelector } from './state/issues'
import { STAGE_LABELS, StageId } from './state/stageIds'
import { targetIssues } from './state/target'
import { REVIEW_COMING_IN, Review } from './stages/Review'
import { Target } from './stages/Target'

/** One rail entry; a stage without a component renders the placeholder naming `comingIn`, Review its own fallback. */
export interface StageDef {
  id: StageId
  label: string
  component?: ComponentType
  comingIn?: string
  selector?: IssueSelector
}

/** The rail in journey order. */
export const STAGES: readonly StageDef[] = [
  { id: StageId.Target, label: STAGE_LABELS[StageId.Target], component: Target, selector: targetIssues },
  { id: StageId.Role, label: STAGE_LABELS[StageId.Role], comingIn: 'Epic 2 — Role presets' },
  { id: StageId.Foundations, label: STAGE_LABELS[StageId.Foundations], comingIn: 'Epic 3 — Cross-cutting foundations' },
  { id: StageId.Infrastructure, label: STAGE_LABELS[StageId.Infrastructure], comingIn: 'Epic 4 — Integrating with what is already there' },
  { id: StageId.Features, label: STAGE_LABELS[StageId.Features], comingIn: 'Epic 5 — Features and their configuration' },
  { id: StageId.Configuration, label: STAGE_LABELS[StageId.Configuration], comingIn: 'Epic 5 — Features and their configuration' },
  { id: StageId.Review, label: STAGE_LABELS[StageId.Review], component: Review, comingIn: REVIEW_COMING_IN },
  { id: StageId.Install, label: STAGE_LABELS[StageId.Install], comingIn: 'Epic 10 — Installing into a cluster' },
]

/** The stage `offset` steps from `id` in `stages`, or undefined past either end. */
export function neighbour(stages: readonly StageDef[], id: StageId, offset: number): StageDef | undefined {
  const index = stages.findIndex((stage) => stage.id === id)
  return index < 0 ? undefined : stages[index + offset]
}
