/** The stage rail: every stage of the wizard in order, and what renders it. */
import type { ComponentType } from 'react'
import type { IssueSelector } from './state/issues'
import { StageId } from './state/stageIds'
import { Target } from './stages/Target'

/** One rail entry; a stage without a component renders the placeholder naming `comingIn`. */
export interface StageDef {
  id: StageId
  label: string
  component?: ComponentType
  comingIn?: string
  selector?: IssueSelector
}

/** The rail in journey order. */
export const STAGES: readonly StageDef[] = [
  { id: StageId.Target, label: 'Target', component: Target },
  { id: StageId.Role, label: 'Role', comingIn: 'Epic 2 — Role presets' },
  { id: StageId.Foundations, label: 'Foundations', comingIn: 'Epic 3 — Cross-cutting foundations' },
  { id: StageId.Infrastructure, label: 'Infrastructure', comingIn: 'Epic 4 — Integrating with what is already there' },
  { id: StageId.Features, label: 'Features', comingIn: 'Epic 5 — Features and their configuration' },
  { id: StageId.Configuration, label: 'Configuration', comingIn: 'Epic 5 — Features and their configuration' },
  { id: StageId.Review, label: 'Review', comingIn: 'Epic 6 — Validation and review' },
  { id: StageId.Install, label: 'Install', comingIn: 'Epic 10 — Installing into a cluster' },
]

/** The stage `offset` steps from `id` in `stages`, or undefined past either end. */
export function neighbour(stages: readonly StageDef[], id: StageId, offset: number): StageDef | undefined {
  const index = stages.findIndex((stage) => stage.id === id)
  return index < 0 ? undefined : stages[index + offset]
}
