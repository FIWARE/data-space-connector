/** S3 Foundations: the answers asked once and used everywhere, their fan-out, and what an edit re-renders. */
import { useEffect, useId, useRef, useState } from 'react'
import { fetchFoundations, type Foundation } from '../api/foundations'
import { toProfile } from '../api/profile'
import { fetchRender, type Change, type ChangeGroup } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { PUBLIC_NAMES_URL, repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import { answerFieldId, fieldElementId, fieldStage } from '../state/openQuestions'
import { StageId } from '../state/stageIds'
import { DerivedAnswers } from './DerivedAnswers'
import { derivedValue, usedByText } from './foundationText'

/** Pause after the last keystroke before the answers are rendered again. */
const RENDER_DEBOUNCE_MS = 300

/** The id prefix of the Organization panel's answers; every other foundation belongs to Network. */
const ORGANIZATION_PREFIX = 'org.'

export const FOUNDATIONS_HEADING = 'Foundations — asked once, used everywhere'
export const ORGANIZATION_PANEL = 'Organization'
export const NETWORK_PANEL = 'Network'
export const HOSTS_HEADING = 'Host names, derived from the base domain'
export const HOSTS_PENDING = 'Derived once the base domain is answered.'
export const HOSTS_EDIT_HINT = 'Editing a single host comes with a later release.'
export const RESOLVE_WARNING = '⚠ These names are called from inside the cluster as well as from outside. Check that they resolve to the ingress from a pod.'
export const RESOLVE_LINK = 'Why, and how ↗'
export const DIFF_HEADING = 'What the last edit changed'
export const PINNED_LABEL = 'pinned, not updated'
export const LOADED_HINT = 'Found in the loaded file, which keeps them while the fields above are unanswered.'
const NOT_SET = '—'

function shown(value: unknown): string {
  if (value === undefined) return NOT_SET
  return typeof value === 'string' ? value : JSON.stringify(value)
}

/** The "used by" button and, on demand, the paths it stands for. */
function UsedBy({ foundation }: { foundation: Foundation }) {
  const [expanded, setExpanded] = useState(false)
  const listId = useId()
  if (foundation.count === 0) return <span className="hint">{usedByText(foundation)}</span>
  return (
    <>
      <button type="button" className="link" aria-expanded={expanded} aria-controls={listId} onClick={() => setExpanded(!expanded)}>
        {usedByText(foundation)}
      </button>
      {expanded && (
        <ul id={listId} className="targets" aria-label={`Where ${foundation.label} is used`}>
          {foundation.targets.map((t) => (
            <li key={t.path}>
              <code>{t.path}</code>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

function DocLink({ foundation }: { foundation: Foundation }) {
  if (!foundation.doc) return null
  return (
    <a href={repositoryDocUrl(foundation.doc)} target="_blank" rel="noreferrer">
      About {foundation.label} ↗
    </a>
  )
}

/** One answer the user gives: input, help, doc link, "ask later" and its fan-out. */
function AnswerField({ foundation }: { foundation: Foundation }) {
  const { state, dispatch } = useStore()
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  return (
    <div className="row">
      <label htmlFor={id}>{foundation.label}</label>
      <input
        id={id}
        value={state.answers.values[foundation.id] ?? ''}
        aria-describedby={`${id}-help`}
        onChange={(e) => dispatch({ type: 'setAnswer', id: foundation.id, value: e.target.value })}
      />
      <AskLater field={field} />
      <p className="hint" id={`${id}-help`}>
        {foundation.help} <DocLink foundation={foundation} /> <UsedBy foundation={foundation} />
      </p>
    </div>
  )
}

/** The derived hosts, read-only with the value they render to. */
function Hosts({ hosts }: { hosts: readonly Foundation[] }) {
  const { state } = useStore()
  return (
    <div role="group" aria-labelledby="foundations-hosts">
      <h4 id="foundations-hosts">{HOSTS_HEADING}</h4>
      <table className="hosts">
        <tbody>
          {hosts.map((h) => (
            <tr key={h.id}>
              <th scope="row">{h.label}</th>
              <td>{derivedValue(h, state.answers.values) ?? <span className="hint">{HOSTS_PENDING}</span>}</td>
              <td>
                <DocLink foundation={h} /> <UsedBy foundation={h} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="hint">{HOSTS_EDIT_HINT}</p>
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

/** The grouped diff of the last render against the one before it. */
function RenderDiff({ groups, foundations }: { groups: readonly ChangeGroup[]; foundations: readonly Foundation[] }) {
  if (groups.length === 0) return null
  return (
    <div className="panel" role="group" aria-labelledby="foundations-diff">
      <h3 id="foundations-diff">{DIFF_HEADING}</h3>
      {groups.map((g) => (
        <div key={g.foundation} role="group" aria-label={foundations.find((f) => f.id === g.foundation)?.label ?? g.foundation}>
          <h4>{foundations.find((f) => f.id === g.foundation)?.label ?? g.foundation}</h4>
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

/** Renders the answers whenever they change (debounced) and keeps the diff; the render on entry is the baseline. */
function useRenderDiff(): { groups: readonly ChangeGroup[]; error?: string } {
  const { state } = useStore()
  const values = state.answers.values
  const latest = useRef(state)
  const entry = useRef(values)
  const [result, setResult] = useState<{ groups: readonly ChangeGroup[]; error?: string }>({ groups: [] })

  useEffect(() => {
    latest.current = state
  }, [state])

  useEffect(() => {
    const controller = new AbortController()
    const baseline = values === entry.current
    const timer = setTimeout(
      () =>
        fetchRender(toProfile(latest.current), latest.current.loaded?.sha256, controller.signal).then(
          (r) => setResult((prev) => ({ groups: baseline ? prev.groups : r.diff.groups })),
          (err: Error) => controller.signal.aborted || setResult((prev) => ({ groups: prev.groups, error: err.message })),
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

  // A Review link may arrive before the foundations, so the field is focused once its input exists.
  useEffect(() => {
    if (!state.focus || fieldStage(state.focus) !== StageId.Foundations) return
    const input = document.getElementById(fieldElementId(state.focus))
    if (!input) return
    input.scrollIntoView({ block: 'center' })
    input.focus({ preventScroll: true })
    dispatch({ type: 'focusDone' })
  }, [state.focus, state.foundations, dispatch])

  const foundations = state.foundations ?? []
  const given = foundations.filter((f) => f.default === '')
  const organization = given.filter((f) => f.id.startsWith(ORGANIZATION_PREFIX))
  const network = given.filter((f) => !f.id.startsWith(ORGANIZATION_PREFIX))
  const hosts = foundations.filter((f) => f.default !== '')

  return (
    <section>
      <h1>{FOUNDATIONS_HEADING}</h1>
      {error && <p className="field-error">{error}</p>}
      {organization.length > 0 && (
        <div className="panel" role="group" aria-labelledby="foundations-organization">
          <h3 id="foundations-organization">{ORGANIZATION_PANEL}</h3>
          {organization.map((f) => (
            <AnswerField key={f.id} foundation={f} />
          ))}
        </div>
      )}
      {network.length + hosts.length > 0 && (
        <div className="panel" role="group" aria-labelledby="foundations-network">
          <h3 id="foundations-network">{NETWORK_PANEL}</h3>
          {network.map((f) => (
            <AnswerField key={f.id} foundation={f} />
          ))}
          {hosts.length > 0 && <Hosts hosts={hosts} />}
        </div>
      )}
      {diff.error && (
        <p className="field-error" role="alert">
          {diff.error}
        </p>
      )}
      <RenderDiff groups={diff.groups} foundations={foundations} />
      <DerivedAnswers stage={StageId.Foundations} hint={LOADED_HINT} />
    </section>
  )
}
