/** Stage 4's state over `profile.infrastructure`: the active mode, which questions it asks and their resolved values. */
import type { Foundation } from '../api/foundations'
import type { Dependency, InfraQuestion, Mode } from '../api/infrastructure'
import type { AnswerValue, InfraChoice } from './answers'
import { checkAnswer, fill, resolvedAnswer, textAnswer, validAnswers, type ReservedRefs } from './foundations'
import { infraModeFieldId, infraQuestionFieldId, type InfraFieldId } from './openQuestions'

/** What an infrastructure default may refer to: the Stage 3 answers the render is sent and the reserved refs. */
export interface InfraContext {
  values: Record<string, AnswerValue>
  foundations: readonly Foundation[]
  reserved: ReservedRefs
}

/** The context the defaults resolve in; only well-formed Stage 3 answers count, as only those are rendered. */
export function infraContext(values: Record<string, AnswerValue>, foundations: readonly Foundation[] = [], reserved: ReservedRefs): InfraContext {
  return { values: validAnswers(values, foundations, reserved), foundations, reserved }
}

const NO_CHOICE: InfraChoice = { mode: '', answers: {} }

/** The stored choice of `dependency`; a never-touched dependency has no mode and no answers. */
export function choiceOf(infrastructure: Record<string, InfraChoice>, dependency: string): InfraChoice {
  return infrastructure[dependency] ?? NO_CHOICE
}

/** The chosen mode, or undefined while none (or one the dependency does not offer) is chosen. */
export function activeMode(dep: Dependency, choice: InfraChoice): Mode | undefined {
  return dep.modes.find((m) => m.id === choice.mode)
}

/** The question `id` of any mode of `dep`; question ids are unique across a dependency's modes. */
export function questionOf(dep: Dependency, id: string): InfraQuestion | undefined {
  return dep.modes.flatMap((m) => m.questions).find((q) => q.id === id)
}

/** The value the question's default derives to; undefined without a default or while a reference is unresolved. */
export function derivedValue(q: InfraQuestion, ctx: InfraContext): string | undefined {
  return q.default === '' ? undefined : fill(q.default, ctx.values, ctx.foundations, ctx.reserved)
}

/** The given answer, else the derived one; a read-only question never takes a given answer. */
export function questionValue(q: InfraQuestion, choice: InfraChoice, ctx: InfraContext): string | undefined {
  const given = q.readOnly ? undefined : textAnswer(choice.answers[q.id])
  return given ?? derivedValue(q, ctx)
}

/** The questions `mode` asks right now: those whose `when` holds over the resolved answers of the same mode, and
 *  per-ingress ones only while their ingress's owner resolves, as only then does the answer render. */
export function shownQuestions(mode: Mode, choice: InfraChoice, ctx: InfraContext): InfraQuestion[] {
  return mode.questions.filter((q) => whenHolds(mode, q, choice, ctx) && ownerResolves(q, ctx))
}

/** The per-ingress questions of `mode` held back because their ingress's owner (a Stage 3 host) is unresolved. */
export function pendingIngressQuestions(mode: Mode, choice: InfraChoice, ctx: InfraContext): InfraQuestion[] {
  return mode.questions.filter((q) => whenHolds(mode, q, choice, ctx) && !ownerResolves(q, ctx))
}

function whenHolds(mode: Mode, q: InfraQuestion, choice: InfraChoice, ctx: InfraContext): boolean {
  if (!q.when) return true
  const other = mode.questions.find((o) => o.id === q.when?.question)
  return other !== undefined && questionValue(other, choice, ctx) === q.when.equals
}

function ownerResolves(q: InfraQuestion, ctx: InfraContext): boolean {
  return !q.ingress || resolvedAnswer(q.ingress.owner, ctx.values, ctx.foundations, ctx.reserved) !== undefined
}

/** The format error of every given answer of a known question, by field id, inactive modes included. */
export function infraErrors(infrastructure: Record<string, InfraChoice>, dependencies: readonly Dependency[] = []): Record<InfraFieldId, string> {
  const errors: Record<InfraFieldId, string> = {}
  for (const dep of dependencies) {
    for (const [id, value] of Object.entries(choiceOf(infrastructure, dep.id).answers)) {
      const q = questionOf(dep, id)
      const error = q && checkAnswer(q.type, value, q.options)
      if (error) errors[infraQuestionFieldId(dep.id, id)] = error
    }
  }
  return errors
}

/** The format errors of the active mode's shown questions; an inactive or hidden answer is not rendered. */
export function shownInfraErrors(infrastructure: Record<string, InfraChoice>, dependencies: readonly Dependency[] = [], ctx: InfraContext): Record<InfraFieldId, string> {
  const errors = infraErrors(infrastructure, dependencies)
  const shown = new Set(shownFields(infrastructure, dependencies, ctx))
  return Object.fromEntries(Object.entries(errors).filter(([field]) => shown.has(field as InfraFieldId)))
}

/** `infrastructure` without the answers that have a format error, so one bad field does not make every profile
 *  request a 422; unchanged while the dependencies are not loaded. */
export function validInfrastructure(infrastructure: Record<string, InfraChoice>, dependencies?: readonly Dependency[]): Record<string, InfraChoice> {
  if (!dependencies) return infrastructure
  const errors = infraErrors(infrastructure, dependencies)
  return Object.fromEntries(
    Object.entries(infrastructure).map(([dep, choice]) => [
      dep,
      { ...choice, answers: Object.fromEntries(Object.entries(choice.answers).filter(([q]) => !(infraQuestionFieldId(dep, q) in errors))) },
    ]),
  )
}

/** Every Stage 4 field asked right now, in screen order: each mode, then its active mode's shown questions. */
export function shownFields(infrastructure: Record<string, InfraChoice>, dependencies: readonly Dependency[], ctx: InfraContext): InfraFieldId[] {
  return dependencies.flatMap((dep) => {
    const choice = choiceOf(infrastructure, dep.id)
    const mode = activeMode(dep, choice)
    const questions = mode ? shownQuestions(mode, choice, ctx) : []
    return [infraModeFieldId(dep.id), ...questions.map((q) => infraQuestionFieldId(dep.id, q.id))]
  })
}

/** The Stage 4 fields that are unanswered: an unchosen mode, a shown question with neither a given nor a derived answer.
 *  A read-only question cannot be answered here, so only its marker counts it; its unresolved ref is counted where asked. */
export function unansweredInfra(infrastructure: Record<string, InfraChoice>, dependencies: readonly Dependency[], ctx: InfraContext): InfraFieldId[] {
  return dependencies.flatMap((dep) => {
    const choice = choiceOf(infrastructure, dep.id)
    const mode = activeMode(dep, choice)
    if (!mode) return [infraModeFieldId(dep.id)]
    return shownQuestions(mode, choice, ctx)
      .filter((q) => !q.readOnly && questionValue(q, choice, ctx) === undefined)
      .map((q) => infraQuestionFieldId(dep.id, q.id))
  })
}

/** `infrastructure` with `dependency`'s mode set; the answers of every mode are kept for a switch back. */
export function setInfraMode(infrastructure: Record<string, InfraChoice>, dependency: string, mode: string): Record<string, InfraChoice> {
  return { ...infrastructure, [dependency]: { ...choiceOf(infrastructure, dependency), mode } }
}

/** `infrastructure` with a question answered; an empty value is no answer, so the question's default applies again. */
export function setInfraAnswer(
  infrastructure: Record<string, InfraChoice>,
  dependency: string,
  question: string,
  value: AnswerValue,
): Record<string, InfraChoice> {
  const choice = choiceOf(infrastructure, dependency)
  const answers = Object.fromEntries(Object.entries(choice.answers).filter(([id]) => id !== question))
  if (value !== '') answers[question] = value
  return { ...infrastructure, [dependency]: { ...choice, answers } }
}

/** `infrastructure` without the dependencies, modes and questions `dependencies` does not declare; unchanged when
 *  nothing goes. */
export function declaredInfrastructure(infrastructure: Record<string, InfraChoice>, dependencies: readonly Dependency[]): Record<string, InfraChoice> {
  let pruned = false
  const kept: Record<string, InfraChoice> = {}
  for (const [id, choice] of Object.entries(infrastructure)) {
    const dep = dependencies.find((d) => d.id === id)
    if (!dep) {
      pruned = true
      continue
    }
    const answers = Object.fromEntries(Object.entries(choice.answers).filter(([q]) => questionOf(dep, q) !== undefined))
    const mode = activeMode(dep, choice) ? choice.mode : ''
    pruned ||= mode !== choice.mode || Object.keys(answers).length !== Object.keys(choice.answers).length
    kept[id] = { mode, answers }
  }
  return pruned ? kept : infrastructure
}
