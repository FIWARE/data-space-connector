/** S3 Foundations: the answers asked once and used everywhere, their fan-out, and what an edit re-renders. */
import { useEffect, useRef, useState } from 'react'
import { fetchFoundations, type Foundation } from '../api/foundations'
import { toProfile } from '../api/profile'
import { fetchRender, type AnswerDiff, type Change, type ChangeGroup, type Finding } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { PUBLIC_NAMES_URL } from '../links'
import { useStore } from '../state/context'
import { answerErrors, defaultApplies, defaultValue, featureOff, resolvedAnswer, textAnswer, validAnswers } from '../state/foundations'
import { answerFieldId, fieldElementId, fieldStage } from '../state/openQuestions'
import { StageId } from '../state/stageIds'
import { AnswerField, FieldNotes } from './AnswerField'
import { DataSpacePanel } from './DataSpacePanel'
import { DerivedAnswers } from './DerivedAnswers'
import { describedBy } from './foundationText'
import { PINNED_LABEL, wouldDerive } from './handSetText'

/** Pause after the last keystroke before the answers are rendered again. */
export const RENDER_DEBOUNCE_MS = 300

export const FOUNDATIONS_HEADING = 'Foundations — asked once, used everywhere'
export const ORGANIZATION_PANEL = 'Organization'
export const NETWORK_PANEL = 'Network'
export const HOSTS_HEADING = 'Host names, derived from the base domain; each can be overridden'
export const HOSTS_PENDING = 'Derived once the base domain is answered.'
export const DERIVED_PENDING = 'Derived once the answers it comes from are given.'
export const DERIVED_BADGE = 'derived'
export const EDIT_LABEL = 'Edit'
/** Badge of a given answer that no longer follows the answers it would derive from. */
export const PINNED_BADGE = 'pinned'
export const RESOLVE_WARNING = '⚠ These names are called from inside the cluster as well as from outside. Check that they resolve to the ingress from a pod.'
export const RESOLVE_LINK = 'Why, and how ↗'
export const DIFF_HEADING = 'What the last edit changed'
export const REDERIVED_HEADING = 'Re-derived'
export const PINNED_HEADING = 'Pinned, not updated'
export const LOADED_HINT = 'Found in the loaded file, which keeps them while the fields above are unanswered.'
const NOT_SET = '—'
const NO_DERIVED_CHANGE: AnswerDiff = { rederived: [], pinned: [] }

function shown(value: unknown): string {
  if (value === undefined || value === null || value === '') return NOT_SET
  return typeof value === 'string' ? value : JSON.stringify(value)
}

/** A choice as a radio group, each option with its note; the default option is checked while none is given. */
function ChoiceField({ foundation, findings }: { foundation: Foundation; findings: readonly Finding[] }) {
  const { state, dispatch } = useStore()
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const chosen = resolvedAnswer(foundation.id, state.answers.values, state.foundations ?? [])
  const own = findings.filter((f) => f.field === field)
  return (
    // tabIndex lets a Review link focus the group.
    <fieldset className="row" id={id} tabIndex={-1} aria-describedby={describedBy(id, undefined, own)}>
      <legend>{foundation.label}</legend>
      {foundation.options.map((o, i) => (
        <div key={o.value} className="choice">
          <input
            type="radio"
            id={`${id}-option-${i}`}
            name={id}
            value={o.value}
            checked={chosen === o.value}
            aria-describedby={o.note ? `${id}-option-${i}-note` : undefined}
            onChange={() => dispatch({ type: 'setAnswer', id: foundation.id, value: o.value })}
          />
          <label htmlFor={`${id}-option-${i}`}>{o.label}</label>
          {o.note && (
            <span className="hint" id={`${id}-option-${i}-note`}>
              {o.note}
            </span>
          )}
        </div>
      ))}
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} own={own} />
    </fieldset>
  )
}

/** An answer with a default: the derived value with "Edit" while none is given, else the input with "Reset to derived";
 *  a given answer is pinned. Where the default does not apply (e.g. its condition fails) the input is asked for directly. */
function DerivedField({ foundation, findings, pending = DERIVED_PENDING }: { foundation: Foundation; findings: readonly Finding[]; pending?: string }) {
  const { state, dispatch } = useStore()
  const [editing, setEditing] = useState(false)
  // Text typed while editing that is not an answer (yet): empty. Undefined shows the derived value.
  const [draft, setDraft] = useState<string | undefined>(undefined)
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const foundations = state.foundations ?? []
  const given = textAnswer(state.answers.values[foundation.id])
  // Only what the render is sent, so an invalid answer it comes from shows it as pending rather than a value never written.
  const values = validAnswers(state.answers.values, foundations)
  const applies = defaultApplies(foundation, values, foundations)
  const derived = defaultValue(foundation, values, foundations)

  useEffect(() => {
    if (editing) document.getElementById(id)?.focus()
  }, [editing, id])
  // A switch away from the default ends the edit, so switching back shows the derived value again.
  const [appliedBefore, setAppliedBefore] = useState(applies)
  if (applies !== appliedBefore) {
    setAppliedBefore(applies)
    if (!applies) {
      setEditing(false)
      setDraft(undefined)
    }
  }

  if (!applies) return <AnswerField foundation={foundation} findings={findings} />
  if (given !== undefined || editing) {
    const reset = () => {
      setEditing(false)
      setDraft(undefined)
      dispatch({ type: 'setAnswer', id: foundation.id, value: '' })
    }
    // Only text that differs from the derived value is an edit, so opening the input or typing the derived value back
    // leaves the answer derived and it keeps following the answers it comes from.
    const change = (value: string) => {
      const same = value === derived
      setEditing(true)
      setDraft(same ? undefined : value)
      dispatch({ type: 'setAnswer', id: foundation.id, value: same ? '' : value })
    }
    const text = given ?? draft ?? derived ?? ''
    return (
      <AnswerField foundation={foundation} findings={findings} reset={reset} value={text} onChange={change}>
        {given !== undefined && <span className="deliberate">{PINNED_BADGE}</span>}
        {given !== undefined && derived !== undefined && derived !== given && <span className="hint">{wouldDerive(derived)}</span>}
      </AnswerField>
    )
  }
  const edit = () => setEditing(true)
  const own = findings.filter((f) => f.field === field)
  return (
    <div className="row">
      <label htmlFor={id}>{foundation.label}</label>
      {/* tabIndex lets a Review link focus the value while it is derived. */}
      <output id={id} tabIndex={-1} aria-describedby={describedBy(id, undefined, own)}>
        {derived ?? <span className="hint">{pending}</span>}
      </output>
      {derived !== undefined && <span className="deliberate">{DERIVED_BADGE}</span>}
      <button type="button" className="link" aria-label={`${EDIT_LABEL} ${foundation.label}`} onClick={edit}>
        {EDIT_LABEL}
      </button>
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} own={own} />
    </div>
  )
}

/** The field a foundation of the Organization panel is asked with. */
function OrganizationField({ foundation, findings }: { foundation: Foundation; findings: readonly Finding[] }) {
  if (foundation.type === 'choice') return <ChoiceField foundation={foundation} findings={findings} />
  if (foundation.default !== '') return <DerivedField foundation={foundation} findings={findings} />
  return <AnswerField foundation={foundation} findings={findings} />
}

/** The derived hosts, each editable (pinned) and resettable. A host with a feature off is hidden unless it is pinned,
 *  marked for later or named by a finding, so Reset and the links from Review still reach it. Once shown, it stays
 *  for the visit, so Reset can show the value it derives and clearing the input does not unmount it mid-edit. */
function Hosts({ hosts, findings }: { hosts: readonly Foundation[]; findings: readonly Finding[] }) {
  const { state } = useStore()
  const [kept, setKept] = useState<ReadonlySet<string>>(() => new Set())
  const exposed = (h: Foundation) => {
    const field = answerFieldId(h.id)
    return (
      state.answers.values[h.id] !== undefined ||
      state.open.includes(field) ||
      findings.some((f) => f.field === field || f.related === field)
    )
  }
  const visible = hosts.filter((h) => !featureOff(h, state.answers) || kept.has(h.id) || exposed(h))
  const newlyKept = visible.filter((h) => featureOff(h, state.answers) && !kept.has(h.id)).map((h) => h.id)
  if (newlyKept.length > 0) setKept(new Set([...kept, ...newlyKept]))
  return (
    <div role="group" aria-labelledby="foundations-hosts">
      <h4 id="foundations-hosts">{HOSTS_HEADING}</h4>
      {visible.map((h) => (
        <DerivedField key={h.id} foundation={h} findings={findings} pending={HOSTS_PENDING} />
      ))}
      <p className="hint">
        {RESOLVE_WARNING}{' '}
        <a href={PUBLIC_NAMES_URL} target="_blank" rel="noreferrer">
          {RESOLVE_LINK}
        </a>
      </p>
    </div>
  )
}

function ChangeItem({ change }: { change: Change }) {
  if (change.status !== 'pinned') {
    return (
      <li>
        <code>{change.path}</code> {shown(change.before)} → {shown(change.after)}
      </li>
    )
  }
  return (
    <li className="pinned">
      <code>{change.path}</code> <span className="deliberate">{PINNED_LABEL}</span>
      {'kept' in change && <> keeps {shown(change.kept)},</>} would have become {shown(change.after)}
    </li>
  )
}

function labelOf(foundations: readonly Foundation[], id: string): string {
  return foundations.find((f) => f.id === id)?.label ?? id
}

/** The derived answers the last edit moved, then the given ones it left: *pinned, not updated*. */
function DerivedDiff({ derived, foundations }: { derived: AnswerDiff; foundations: readonly Foundation[] }) {
  return (
    <>
      {derived.rederived.length > 0 && (
        <div role="group" aria-labelledby="foundations-rederived">
          <h4 id="foundations-rederived">{REDERIVED_HEADING}</h4>
          <ul>
            {derived.rederived.map((c) => (
              <li key={c.id}>
                {labelOf(foundations, c.id)}: {shown(c.before)} → {shown(c.after)}
              </li>
            ))}
          </ul>
        </div>
      )}
      {derived.pinned.length > 0 && (
        <div role="group" aria-labelledby="foundations-pinned">
          <h4 id="foundations-pinned">{PINNED_HEADING}</h4>
          <ul>
            {derived.pinned.map((p) => (
              <li key={p.id} className="pinned">
                {labelOf(foundations, p.id)}: keeps {shown(p.kept)}, would have become {shown(p.after)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  )
}

/** The diff of the last render against the one before it: derived answers first, then the leaves by foundation. */
function RenderDiff({ groups, derived, foundations }: { groups: readonly ChangeGroup[]; derived: AnswerDiff; foundations: readonly Foundation[] }) {
  if (groups.length + derived.rederived.length + derived.pinned.length === 0) return null
  return (
    <div className="panel" role="group" aria-labelledby="foundations-diff">
      <h3 id="foundations-diff">{DIFF_HEADING}</h3>
      <DerivedDiff derived={derived} foundations={foundations} />
      {groups.map((g) => (
        <div key={g.foundation} role="group" aria-label={labelOf(foundations, g.foundation)}>
          <h4>{labelOf(foundations, g.foundation)}</h4>
          <ul>
            {g.changes.map((c) => (
              <ChangeItem key={c.path} change={c} />
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}

/** What the last render of the answers left: its diffs (none for the baseline), its findings and a refusal. */
interface RenderState {
  groups: readonly ChangeGroup[]
  derived: AnswerDiff
  findings: readonly Finding[]
  error?: string
}

/** Renders the answers whenever they change (debounced) and keeps the diff and findings; the first render is the baseline.
 *  Nothing is rendered while an answer is invalid, so the server's baseline stays the last valid tree. */
function useRenderDiff(): RenderState {
  const { state } = useStore()
  const values = state.answers.values
  const latest = useRef(state)
  const started = useRef(false)
  const [result, setResult] = useState<RenderState>({ groups: [], derived: NO_DERIVED_CHANGE, findings: [] })

  useEffect(() => {
    latest.current = state
  }, [state])

  useEffect(() => {
    if (Object.keys(answerErrors(values, latest.current.foundations)).length > 0) return
    const controller = new AbortController()
    const baseline = !started.current
    const timer = setTimeout(
      () =>
        fetchRender(toProfile(latest.current), latest.current.loaded?.sha256, controller.signal).then(
          (r) => {
            started.current = true
            setResult((prev) => ({
              groups: baseline ? prev.groups : r.diff.groups,
              derived: baseline ? prev.derived : r.derived,
              findings: r.findings,
            }))
          },
          (err: Error) => controller.signal.aborted || setResult((prev) => ({ ...prev, error: err.message })),
        ),
      baseline ? 0 : RENDER_DEBOUNCE_MS,
    )
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [values])
  return result
}

/** S3 Foundations screen. */
export function Foundations() {
  const { state, dispatch } = useStore()
  const { chart, chartVersion } = state.answers.target
  const [error, setError] = useState<string>()
  const diff = useRenderDiff()

  useEffect(() => {
    const controller = new AbortController()
    fetchFoundations(chart, chartVersion, controller.signal).then(
      (list) => dispatch({ type: 'foundationsLoaded', foundations: list.foundations }),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [chart, chartVersion, dispatch])

  // A Review link may arrive before the foundations, or before the findings that show a hidden host,
  // so the field is focused once its input exists.
  useEffect(() => {
    if (!state.focus || fieldStage(state.focus) !== StageId.Foundations) return
    const input = document.getElementById(fieldElementId(state.focus))
    if (!input) return
    input.scrollIntoView({ block: 'center' })
    input.focus({ preventScroll: true })
    dispatch({ type: 'focusDone' })
  }, [state.focus, state.foundations, diff.findings, dispatch])

  const foundations = state.foundations ?? []
  const organization = foundations.filter((f) => f.group === 'organization')
  const dataspace = foundations.filter((f) => f.group === 'dataspace')
  const network = foundations.filter((f) => f.group === 'network' && f.default === '')
  const hosts = foundations.filter((f) => f.group === 'network' && f.default !== '')

  return (
    <section>
      <h1>{FOUNDATIONS_HEADING}</h1>
      {error && <p className="field-error">{error}</p>}
      {organization.length > 0 && (
        <div className="panel" role="group" aria-labelledby="foundations-organization">
          <h3 id="foundations-organization">{ORGANIZATION_PANEL}</h3>
          {organization.map((f) => (
            <OrganizationField key={f.id} foundation={f} findings={diff.findings} />
          ))}
        </div>
      )}
      {dataspace.length > 0 && <DataSpacePanel foundations={dataspace} findings={diff.findings} />}
      {network.length + hosts.length > 0 && (
        <div className="panel" role="group" aria-labelledby="foundations-network">
          <h3 id="foundations-network">{NETWORK_PANEL}</h3>
          {network.map((f) => (
            <AnswerField key={f.id} foundation={f} findings={diff.findings} />
          ))}
          {hosts.length > 0 && <Hosts hosts={hosts} findings={diff.findings} />}
        </div>
      )}
      {diff.error && (
        <p className="field-error" role="alert">
          {diff.error}
        </p>
      )}
      <RenderDiff groups={diff.groups} derived={diff.derived} foundations={foundations} />
      <DerivedAnswers stage={StageId.Foundations} hint={LOADED_HINT} />
    </section>
  )
}
