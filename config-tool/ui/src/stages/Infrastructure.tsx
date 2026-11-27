/** S4 Infrastructure: per dependency, how it is provided (three modes) and what the chosen mode asks (ADR 0026). */
import { useEffect, useRef, useState } from 'react'
import { fetchFoundations, type Foundation } from '../api/foundations'
import { fetchInfrastructure, type Dependency, type InfraQuestion, type Mode } from '../api/infrastructure'
import { toProfile } from '../api/profile'
import { fetchRender, type Finding } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { repositoryDocUrl } from '../links'
import type { InfraChoice } from '../state/answers'
import { useStore } from '../state/context'
import { checkAnswer, reservedRefs, textAnswer } from '../state/foundations'
import { activeMode, choiceOf, derivedValue, infraContext, questionValue, shownQuestions, type InfraContext } from '../state/infrastructure'
import { fieldElementId, fieldStage, infraModeFieldId, infraQuestionFieldId } from '../state/openQuestions'
import { StageId } from '../state/stageIds'
import { AnswerField, ChoiceField, FieldNotes, type FieldBinding } from './AnswerField'
import { FindingNote } from './Findings'
import { describedBy, findingNoteId } from './foundationText'
import { RENDER_DEBOUNCE_MS } from './Foundations'
import {
  DERIVED_BADGE,
  DERIVED_PENDING,
  DEVELOPMENT_BADGE,
  INFRASTRUCTURE_HEADING,
  NO_DEPENDENCIES,
  PRODUCTION_LINK,
  SECRETS_NOTE,
  WARNING_LINK,
  writesListLabel,
  writesText,
} from './infrastructureText'

const NO_FINDINGS: readonly Finding[] = []

// The shared field components take a foundation; a question is shown through one that has no keys of its own.
function asFoundation(q: InfraQuestion, label: string, help: string): Foundation {
  return {
    id: q.id,
    stage: StageId.Infrastructure,
    type: q.type,
    group: 'platform',
    default: q.default,
    defaultWhen: null,
    prefix: '',
    options: q.options,
    label,
    help,
    doc: q.doc,
    targets: [],
    count: 0,
    components: [],
    features: [],
  }
}

/** What one question needs from its dependency's state. */
interface QuestionProps {
  dependency: string
  question: InfraQuestion
  choice: InfraChoice
  ctx: InfraContext
  findings: readonly Finding[]
  /** The row label; the question's own unless it is one row of a per-ingress group. */
  label: string
  help: string
}

/** A read-only question: its derived value, never an input. */
function ReadOnlyField({ dependency, question, ctx, findings, label, help }: QuestionProps) {
  const field = infraQuestionFieldId(dependency, question.id)
  const id = fieldElementId(field)
  const own = findings.filter((f) => f.field === field)
  const derived = derivedValue(question, ctx)
  return (
    <div className="row">
      <label htmlFor={id}>{label}</label>
      {/* tabIndex lets a Review link focus the value. */}
      <output id={id} tabIndex={-1} aria-describedby={describedBy(id, undefined, own)}>
        {derived ?? <span className="hint">{DERIVED_PENDING}</span>}
      </output>
      {derived !== undefined && <span className="deliberate">{DERIVED_BADGE}</span>}
      <AskLater field={field} />
      <FieldNotes foundation={asFoundation(question, label, help)} id={id} own={own} />
    </div>
  )
}

/** One question of the active mode, asked with Stage 3's field components. A text question with a default shows the
 *  derived value until an edit differs from it, so it keeps following the answers it comes from. */
function QuestionField(props: QuestionProps) {
  const { dependency, question: q, choice, ctx, findings, label, help } = props
  const { dispatch } = useStore()
  // Text typed that is not an answer (yet): empty, or the derived value typed back. Undefined shows the derived value.
  const [draft, setDraft] = useState<string | undefined>(undefined)
  if (q.readOnly) return <ReadOnlyField {...props} />
  const given = choice.answers[q.id]
  const binding: FieldBinding = {
    field: infraQuestionFieldId(dependency, q.id),
    value: q.type === 'choice' ? questionValue(q, choice, ctx) : textAnswer(given),
    error: given === undefined ? undefined : checkAnswer(q.type, given, q.options),
    set: (value) => dispatch({ type: 'setInfraAnswer', dependency, question: q.id, value }),
  }
  const foundation = asFoundation(q, label, help)
  if (q.type === 'choice') return <ChoiceField foundation={foundation} findings={findings} binding={binding} />
  const derived = derivedValue(q, ctx)
  if (q.default === '') return <AnswerField foundation={foundation} findings={findings} binding={binding} />
  const change = (value: string) => {
    const same = value === derived
    setDraft(same ? undefined : value)
    binding.set(same ? '' : value)
  }
  const reset = () => {
    setDraft(undefined)
    binding.set('')
  }
  return (
    <AnswerField
      foundation={foundation}
      findings={findings}
      binding={binding}
      value={binding.value ?? draft ?? derived ?? ''}
      onChange={change}
      reset={binding.value !== undefined ? reset : undefined}
    >
      {binding.value === undefined && draft === undefined && derived !== undefined && <span className="deliberate">{DERIVED_BADGE}</span>}
    </AnswerField>
  )
}

/** The active mode's shown questions: plain ones first, then each per-ingress question as one group of rows. */
function ModeQuestions({ dep, mode, choice, ctx, findings }: { dep: Dependency; mode: Mode; choice: InfraChoice; ctx: InfraContext; findings: readonly Finding[] }) {
  const shown = shownQuestions(mode, choice, ctx)
  const plain = shown.filter((q) => !q.ingress)
  const perIngress = shown.filter((q) => q.ingress)
  const [group] = perIngress
  const props = (q: InfraQuestion) => ({ dependency: dep.id, question: q, choice, ctx, findings })
  return (
    <>
      {plain.map((q) => (
        <QuestionField key={q.id} {...props(q)} label={q.label} help={q.help} />
      ))}
      {group && (
        <div role="group" aria-labelledby={`${dep.id}-per-ingress`}>
          <h4 id={`${dep.id}-per-ingress`}>{group.label}</h4>
          <p className="hint">
            {group.help} {SECRETS_NOTE}
          </p>
          {perIngress.map((q) => (
            <QuestionField key={q.id} {...props(q)} label={q.ingress?.label ?? q.label} help="" />
          ))}
        </div>
      )}
    </>
  )
}

/** One dependency: its modes as a radio group with their notes, the chosen mode's warning, questions and keys. */
function DependencyCard({ dep, ctx, findings }: { dep: Dependency; ctx: InfraContext; findings: readonly Finding[] }) {
  const { state, dispatch } = useStore()
  const choice = choiceOf(state.answers.infrastructure, dep.id)
  const mode = activeMode(dep, choice)
  const field = infraModeFieldId(dep.id)
  const id = fieldElementId(field)
  const legendId = `${id}-legend`
  const own = findings.filter((f) => f.field === field)
  return (
    <section className="panel" aria-labelledby={legendId}>
      {/* tabIndex lets a Review link focus the group. */}
      <fieldset id={id} tabIndex={-1} aria-describedby={describedBy(id, undefined, own)}>
        <legend id={legendId}>{dep.title}</legend>
        {dep.modes.map((m) => {
          const option = `${id}-${m.id}`
          return (
            <div key={m.id} className="choice">
              <input
                type="radio"
                id={option}
                name={id}
                value={m.id}
                checked={choice.mode === m.id}
                aria-describedby={m.note ? `${option}-note` : undefined}
                onChange={() => dispatch({ type: 'setInfraMode', dependency: dep.id, mode: m.id })}
              />
              <label htmlFor={option}>{m.label}</label>
              {m.note && (
                <span className="hint" id={`${option}-note`}>
                  {m.note}
                </span>
              )}
              {m.development && <span className="badge">{DEVELOPMENT_BADGE}</span>}
              {m.development && m.productionDoc && (
                <a href={repositoryDocUrl(m.productionDoc)} target="_blank" rel="noreferrer">
                  {PRODUCTION_LINK}
                </a>
              )}
            </div>
          )
        })}
        <AskLater field={field} />
        {own.map((f) => (
          <FindingNote key={findingNoteId(id, f)} finding={f} id={findingNoteId(id, f)} />
        ))}
        <p className="hint" id={`${id}-help`}>
          {dep.help}{' '}
          {dep.doc && (
            <a href={repositoryDocUrl(dep.doc)} target="_blank" rel="noreferrer">
              About {dep.title} ↗
            </a>
          )}
        </p>
      </fieldset>
      {mode?.warning && (
        <p className="hint" role="note">
          ⚠ {mode.warning}{' '}
          {mode.doc && (
            <a href={repositoryDocUrl(mode.doc)} target="_blank" rel="noreferrer">
              {WARNING_LINK}
            </a>
          )}
        </p>
      )}
      {mode && <ModeQuestions dep={dep} mode={mode} choice={choice} ctx={ctx} findings={findings} />}
      {mode && mode.keys.length > 0 && (
        <details>
          <summary>{writesText(mode.keys.length)}</summary>
          <ul className="targets" aria-label={writesListLabel(mode.label)}>
            {mode.keys.map((k) => (
              <li key={k.path}>
                <code>{k.path}</code>
              </li>
            ))}
          </ul>
        </details>
      )}
    </section>
  )
}

/** The findings of the live render of the whole profile, re-rendered (debounced) whenever an answer changes. */
function useFindings(): { findings: readonly Finding[]; error?: string } {
  const { state } = useStore()
  const latest = useRef(state)
  const started = useRef(false)
  const [result, setResult] = useState<{ findings: readonly Finding[]; error?: string }>({ findings: NO_FINDINGS })
  const { infrastructure, values } = state.answers
  // Until the dependencies are known, toProfile cannot leave out a malformed answer and the server would refuse it.
  const ready = state.dependencies !== undefined

  useEffect(() => {
    latest.current = state
  }, [state])

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    const timer = setTimeout(
      () =>
        fetchRender(toProfile(latest.current), latest.current.loaded?.sha256, controller.signal).then(
          (r) => {
            started.current = true
            setResult({ findings: r.findings })
          },
          (err: Error) => controller.signal.aborted || setResult((prev) => ({ ...prev, error: err.message })),
        ),
      started.current ? RENDER_DEBOUNCE_MS : 0,
    )
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [infrastructure, values, ready])
  return result
}

/** S4 Infrastructure screen. */
export function Infrastructure() {
  const { state, dispatch } = useStore()
  const { chart, chartVersion } = state.answers.target
  const [error, setError] = useState<string>()
  const { findings, error: renderError } = useFindings()
  const foundationsKnown = state.foundations !== undefined

  useEffect(() => {
    const controller = new AbortController()
    fetchInfrastructure(chart, chartVersion, controller.signal).then(
      (list) => dispatch({ type: 'infrastructureLoaded', dependencies: list.dependencies }),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [chart, chartVersion, dispatch])

  // The per-host defaults derive from Stage 3's hosts, so their foundations are needed even if Stage 3 was skipped.
  useEffect(() => {
    if (foundationsKnown) return
    const controller = new AbortController()
    fetchFoundations(chart, chartVersion, controller.signal).then(
      (list) => dispatch({ type: 'foundationsLoaded', foundations: list.foundations }),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [chart, chartVersion, foundationsKnown, dispatch])

  // A Review link may arrive before the dependencies, so the field is focused once its input exists.
  useEffect(() => {
    if (!state.focus || fieldStage(state.focus) !== StageId.Infrastructure) return
    const input = document.getElementById(fieldElementId(state.focus))
    if (!input) return
    input.scrollIntoView({ block: 'center' })
    input.focus({ preventScroll: true })
    dispatch({ type: 'focusDone' })
  }, [state.focus, state.dependencies, state.answers.infrastructure, dispatch])

  const { values, target } = state.answers
  const ctx = infraContext(values, state.foundations, reservedRefs(target))
  const dependencies = state.dependencies
  return (
    <section>
      <h1>{INFRASTRUCTURE_HEADING}</h1>
      {error && <p className="field-error">{error}</p>}
      {dependencies?.length === 0 && <p className="hint">{NO_DEPENDENCIES}</p>}
      {dependencies?.map((dep) => (
        <DependencyCard key={dep.id} dep={dep} ctx={ctx} findings={findings} />
      ))}
      {renderError && (
        <p className="field-error" role="alert">
          {renderError}
        </p>
      )}
    </section>
  )
}
