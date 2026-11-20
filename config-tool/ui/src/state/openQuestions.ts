/** Open questions: fields left unanswered or deliberately marked "ask later", each counted once. */
import { TargetType, type Target } from './answers'
import type { Issues } from './issues'
import type { AppState } from './reducer'
import { StageId } from './stageIds'
import { MSG_REQUIRED, TargetField, validateTarget } from './target'

/** A field an open question can name, `<section>.<key>` as in the profile's `open`. */
export type FieldId = `target.${TargetField}`

type Section = FieldId extends `${infer S}.${string}` ? S : never

const SECTION_STAGES: Record<Section, StageId> = { target: StageId.Target }

/** The field id of a Target field. */
export function targetFieldId(field: TargetField): FieldId {
  return `target.${field}`
}

/** Every field id, in screen order. */
export const FIELD_IDS: readonly FieldId[] = Object.values(TargetField).map(targetFieldId)

/** How Review names each field. */
export const FIELD_LABELS: Record<FieldId, string> = {
  'target.type': 'Target',
  'target.context': 'Kube context',
  'target.namespace': 'Namespace',
  'target.createNamespace': 'Create the namespace',
  'target.releaseName': 'Release name',
  'target.chart': 'Chart',
  'target.chartVersion': 'Chart version',
  'target.environment': 'Environment',
}

/** The stage that asks the field. */
export function fieldStage(id: FieldId): StageId {
  return SECTION_STAGES[id.split('.')[0] as Section]
}

/** DOM id of the field's input, so a Review link can focus it. */
export function fieldElementId(id: FieldId): string {
  return `field-${id}`
}

const CLUSTER_ONLY: readonly FieldId[] = [targetFieldId(TargetField.Context), targetFieldId(TargetField.CreateNamespace)]

// A file target hides the cluster fields, so their markers do not count; they are kept for a switch back.
function applies(id: FieldId, target: Target): boolean {
  return target.type === TargetType.Cluster || !CLUSTER_ONLY.includes(id)
}

/** One open question as the rail counts it and Review lists it. */
export interface OpenQuestion {
  field: FieldId
  stage: StageId
  label: string
  /** Deliberately marked "ask later", as opposed to merely unanswered. */
  marked: boolean
}

/** The union of unanswered and marked fields, in screen order. */
export function openQuestions(state: AppState): OpenQuestion[] {
  const target = state.answers.target
  const unanswered = new Set(
    validateTarget(target)
      .filter((e) => e.message === MSG_REQUIRED)
      .map((e) => targetFieldId(e.field)),
  )
  return FIELD_IDS.filter((id) => unanswered.has(id) || (state.open.includes(id) && applies(id, target))).map((id) => ({
    field: id,
    stage: fieldStage(id),
    label: FIELD_LABELS[id],
    marked: state.open.includes(id),
  }))
}

/** The Target stage's footer and rail counters; a marked field with a format error counts as open and as an error. */
export function targetIssues(state: AppState): Issues {
  const errors = validateTarget(state.answers.target).filter((e) => e.message !== MSG_REQUIRED).length
  const open = openQuestions(state).filter((q) => q.stage === StageId.Target).length
  return { warnings: 0, errors, open }
}

/** The Role stage's counters: no role picked yet is one open question. */
export function roleIssues(state: AppState): Issues {
  return { warnings: 0, errors: 0, open: state.answers.role === undefined ? 1 : 0 }
}
