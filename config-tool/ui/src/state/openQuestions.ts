/** Open questions: fields left unanswered or deliberately marked "ask later", each counted once. */
import type { Foundation } from '../api/foundations'
import { TargetType, type Target } from './answers'
import type { Issues } from './issues'
import type { AppState } from './reducer'
import { StageId } from './stageIds'
import { MSG_REQUIRED, TargetField, validateTarget } from './target'

/** A Target field's id. */
export type TargetFieldId = `target.${TargetField}`

/** The open-question id of a foundation answer, `answers.<dotted id>` as the server names it. */
export type AnswerFieldId = `answers.${string}`

/** A field an open question can name, `<section>.<key>` as in the profile's `open`. */
export type FieldId = TargetFieldId | AnswerFieldId

type Section = FieldId extends `${infer S}.${string}` ? S : never

// Every foundation the shipped bundles declare is asked on Foundations (`stage: foundations`).
const SECTION_STAGES: Record<Section, StageId> = { target: StageId.Target, answers: StageId.Foundations }

const ANSWERS_PREFIX = 'answers.'

/** The field id of a Target field. */
export function targetFieldId(field: TargetField): TargetFieldId {
  return `target.${field}`
}

/** The field id of a foundation answer. */
export function answerFieldId(id: string): AnswerFieldId {
  return `${ANSWERS_PREFIX}${id}`
}

/** Whether `id` is a field id the UI knows: a Target field or any foundation answer (the server checks the id). */
export function isFieldId(id: string): id is FieldId {
  return id.startsWith(ANSWERS_PREFIX) ? id.length > ANSWERS_PREFIX.length : (FIELD_IDS as readonly string[]).includes(id)
}

/** Every Target field id, in screen order. */
export const FIELD_IDS: readonly TargetFieldId[] = Object.values(TargetField).map(targetFieldId)

/** How Review names each Target field. */
export const FIELD_LABELS: Record<TargetFieldId, string> = {
  'target.type': 'Target',
  'target.context': 'Kube context',
  'target.namespace': 'Namespace',
  'target.createNamespace': 'Create the namespace',
  'target.releaseName': 'Release name',
  'target.chart': 'Chart',
  'target.chartVersion': 'Chart version',
  'target.environment': 'Environment',
}

/** The Role stage's question; it has no "ask later" marker, so it is not a `FieldId`. */
export const ROLE_QUESTION = 'role'

/** Anything Review can list as open: a markable field or the role. */
export type QuestionId = FieldId | typeof ROLE_QUESTION

/** The stage that asks the question. */
export function fieldStage(id: QuestionId): StageId {
  return id === ROLE_QUESTION ? StageId.Role : SECTION_STAGES[id.split('.')[0] as Section]
}

/** How Review names a field; an answer takes its foundation's label, or its id until the foundations are loaded. */
export function fieldLabel(id: FieldId, foundations: readonly Foundation[] = []): string {
  if (!id.startsWith(ANSWERS_PREFIX)) return FIELD_LABELS[id as TargetFieldId]
  const answer = id.slice(ANSWERS_PREFIX.length)
  return foundations.find((f) => f.id === answer)?.label ?? answer
}

/** DOM id of the field's input, so a Review link can focus it. */
export function fieldElementId(id: QuestionId): string {
  return `field-${id}`
}

const CLUSTER_ONLY: readonly TargetFieldId[] = [targetFieldId(TargetField.Context), targetFieldId(TargetField.CreateNamespace)]

// A file target hides the cluster fields, so their markers do not count; they are kept for a switch back.
function applies(id: TargetFieldId, target: Target): boolean {
  return target.type === TargetType.Cluster || !CLUSTER_ONLY.includes(id)
}

/** One open question as the rail counts it and Review lists it. */
export interface OpenQuestion {
  field: QuestionId
  stage: StageId
  label: string
  /** Deliberately marked "ask later", as opposed to merely unanswered. */
  marked: boolean
}

const ROLE_OPEN: OpenQuestion = { field: ROLE_QUESTION, stage: StageId.Role, label: 'Role', marked: false }

/** The union of unanswered and marked fields, an unpicked role, then marked answers: stage order. An answer is open
 *  only when marked; unanswered, it renders nothing and the chart default stays. */
export function openQuestions(state: AppState): OpenQuestion[] {
  const target = state.answers.target
  const unanswered = new Set(
    validateTarget(target)
      .filter((e) => e.message === MSG_REQUIRED)
      .map((e) => targetFieldId(e.field)),
  )
  const targetFields = FIELD_IDS.filter((id) => unanswered.has(id) || (state.open.includes(id) && applies(id, target)))
  const question = (id: FieldId): OpenQuestion => ({
    field: id,
    stage: fieldStage(id),
    label: fieldLabel(id, state.foundations),
    marked: state.open.includes(id),
  })
  const role = state.answers.role === undefined ? [ROLE_OPEN] : []
  const answers = state.open.filter((id) => id.startsWith(ANSWERS_PREFIX))
  return [...targetFields.map(question), ...role, ...answers.map(question)]
}

/** The Target stage's footer and rail counters; a marked field with a format error counts as open and as an error. */
export function targetIssues(state: AppState): Issues {
  const errors = validateTarget(state.answers.target).filter((e) => e.message !== MSG_REQUIRED).length
  return { warnings: 0, errors, open: openCount(state, StageId.Target) }
}

/** The Role stage's counters: no role picked yet is one open question. */
export function roleIssues(state: AppState): Issues {
  return { warnings: 0, errors: 0, open: openCount(state, StageId.Role) }
}

/** The Foundations stage's counters: its marked answers. */
export function foundationIssues(state: AppState): Issues {
  return { warnings: 0, errors: 0, open: openCount(state, StageId.Foundations) }
}

function openCount(state: AppState, stage: StageId): number {
  return openQuestions(state).filter((q) => q.stage === stage).length
}
