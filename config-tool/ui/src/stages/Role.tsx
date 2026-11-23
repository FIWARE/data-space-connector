/** S2 Role: a preset that decides which features step 5 starts from (ADR 0017). */
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { toProfile } from '../api/profile'
import { applyPreset, type ChartSwitch, type Role as RoleDef, type RoleCatalogue } from '../api/roles'
import { repositoryDocUrl, SIZING_URL } from '../links'
import { DEFAULT_CHART } from '../state/answers'
import { useStore } from '../state/context'
import { ROLE_QUESTION } from '../state/openQuestions'
import { StageId } from '../state/stageIds'
import { useRoles } from './useRoles'

/** Text under the feature list: a preset never locks anything. */
export const NOTHING_LOCKED = 'Everything else stays available on step 5 — nothing here is locked.'
/** Text of a card whose footprint depends on the features picked (custom). */
export const DEPENDS_ON_SELECTION = 'depends on your selection'

const ARROW_STEP: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }

function recommendedCount(role: RoleDef): number {
  return Object.values(role.features).filter((offer) => offer === 'recommended').length
}

/** One-line footprint of a card: recommended features and the measured floor. */
function footprint(role: RoleDef): string {
  if (!role.floor) return DEPENDS_ON_SELECTION
  const count = recommendedCount(role)
  return `${count} ${count === 1 ? 'feature' : 'features'} · ~${role.floor.memory} memory · ${role.floor.cpu} CPU`
}

/** The inline notice after a role switched the target chart and dropped its version. */
function switchNotice(sw: ChartSwitch): string {
  const from = sw.fromVersion ? `${sw.fromChart} ${sw.fromVersion}` : sw.fromChart
  return `Target chart switched from ${from} to ${sw.toChart} — pick its version on`
}

/** The sentence naming the user toggles a chart switch dropped, by catalogue title; empty when none were. */
function droppedNotice(sw: ChartSwitch, catalogue?: RoleCatalogue): string {
  if (sw.droppedChoices.length === 0) return ''
  const titles = sw.droppedChoices.map((id) => catalogue?.features.find((f) => f.id === id)?.title ?? id)
  return `Your choices for ${titles.join(', ')} do not apply to the ${sw.toChart} chart and were dropped.`
}

/** Why a role that deploys another chart cannot be picked while a values file is loaded. */
function chartLockedReason(role: RoleDef, fileChart: string): string {
  return `${role.title} deploys the ${role.chart} chart; the opened values.yaml is a ${fileChart} file. Start a new configuration to set it up.`
}

/** A role card, one radio of the group; activation is ignored while disabled or a pick is in flight. */
function RoleCard({
  role,
  checked,
  focusable,
  disabledReason,
  onSelect,
  onKeyDown,
  cardRef,
}: {
  role: RoleDef
  checked: boolean
  focusable: boolean
  disabledReason?: string
  onSelect: () => void
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
  cardRef: (el: HTMLButtonElement | null) => void
}) {
  const reasonId = `role-${role.id}-locked`
  return (
    <button
      type="button"
      ref={cardRef}
      role="radio"
      className="card"
      aria-checked={checked}
      aria-disabled={disabledReason !== undefined}
      aria-describedby={disabledReason ? reasonId : undefined}
      tabIndex={focusable ? 0 : -1}
      onClick={onSelect}
      onKeyDown={onKeyDown}
    >
      <h3>
        {role.title} {checked && <span aria-hidden="true">✓</span>}
      </h3>
      <p>{role.summary}</p>
      {role.chart !== DEFAULT_CHART && <p className="hint">chart: {role.chart}</p>}
      <p className="hint">{footprint(role)}</p>
      {disabledReason && (
        <p className="hint" id={reasonId}>
          {disabledReason}
        </p>
      )}
    </button>
  )
}

/** What the selected role turns on, and the toggles the loaded values.yaml kept. */
function Selection({ role, catalogue }: { role: RoleDef; catalogue: RoleCatalogue }) {
  const { state } = useStore()
  const features = state.answers.features
  const titled = catalogue.features.filter((f) => features[f.id])
  const turnedOn = titled.filter((f) => features[f.id]?.source === 'preset' && features[f.id]?.enabled)
  const fromFile = titled.filter((f) => features[f.id]?.source === 'loaded')
  return (
    <div className="panel" aria-label="Selected role">
      <h3>{role.title} turns on</h3>
      <p>{turnedOn.length > 0 ? turnedOn.map((f) => f.title).join(' · ') : 'Nothing is preselected.'}</p>
      {fromFile.length > 0 && (
        <ul className="notes" aria-label="From your values.yaml">
          {fromFile.map((f) => (
            <li key={f.id}>
              {f.title}: {features[f.id]?.enabled ? 'on' : 'off'}, from your values.yaml
            </li>
          ))}
        </ul>
      )}
      <p className="hint">{NOTHING_LOCKED}</p>
      <p className="hint">
        <a href={repositoryDocUrl(role.docs)} target="_blank" rel="noreferrer">
          {role.title} guide ↗
        </a>
      </p>
    </div>
  )
}

/** S2 Role screen. */
export function Role() {
  const { state, dispatch } = useStore()
  const { chart, chartVersion } = state.answers.target
  const { catalogue, error: fetchError } = useRoles(chart, chartVersion)
  const [pending, setPending] = useState<string>()
  const [refused, setRefused] = useState<{ title: string; message: string }>()
  const [focused, setFocused] = useState<number>()
  const cards = useRef<(HTMLButtonElement | null)[]>([])
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const roles = catalogue?.roles ?? []
  const selected = roles.find((r) => r.id === state.answers.role)
  // Classification of the loaded file belongs to its chart, so a role deploying another chart is out of reach.
  const lockedReason = (role: RoleDef) => (state.loaded && role.chart !== chart ? chartLockedReason(role, state.loaded.chart) : undefined)
  // The tab stop is the checked card unless the arrow keys moved focus inside the group.
  const tabStop = focused ?? (selected ? roles.indexOf(selected) : 0)

  useEffect(() => {
    if (state.focus !== ROLE_QUESTION || (roles.length === 0 && !fetchError)) return
    cards.current[tabStop]?.focus()
    dispatch({ type: 'focusDone' })
  }, [state.focus, roles.length, fetchError, tabStop, dispatch])

  const select = async (role: RoleDef) => {
    if (pending || lockedReason(role) || role.id === state.answers.role) return
    setPending(role.id)
    setRefused(undefined)
    try {
      const result = await applyPreset(toProfile(state), role.id, state.loaded?.sha256)
      // A pick that answers after the stage was left must not change a configuration started since.
      if (mounted.current) {
        dispatch({ type: 'roleApplied', profile: result.profile, chartSwitch: result.chartSwitch })
        setFocused(undefined)
      }
    } catch (err) {
      if (mounted.current) setRefused({ title: role.title, message: (err as Error).message })
    } finally {
      if (mounted.current) setPending(undefined)
    }
  }

  const onKeyDown = (index: number) => (event: KeyboardEvent<HTMLButtonElement>) => {
    const step = ARROW_STEP[event.key]
    if (step === undefined || roles.length === 0) return
    event.preventDefault()
    const next = (index + step + roles.length) % roles.length
    setFocused(next)
    cards.current[next]?.focus()
  }

  return (
    <section>
      <h1>What does this organization do in the data space?</h1>
      <p className="lede">A role preselects features. Every one of them stays editable.</p>
      {fetchError && <p className="field-error">{fetchError}</p>}
      {state.chartSwitch && (
        <p className="hint" role="status">
          {switchNotice(state.chartSwitch)}{' '}
          <button type="button" className="link" onClick={() => dispatch({ type: 'goTo', stage: StageId.Target })}>
            step 1
          </button>
          . {droppedNotice(state.chartSwitch, catalogue)}
        </p>
      )}
      <div
        className="cards"
        role="radiogroup"
        aria-label="Role"
        aria-busy={pending !== undefined}
        onBlur={(e) => e.currentTarget.contains(e.relatedTarget) || setFocused(undefined)}
      >
        {roles.map((role, index) => (
          <RoleCard
            key={role.id}
            role={role}
            checked={role.id === state.answers.role}
            focusable={index === tabStop}
            disabledReason={lockedReason(role)}
            onSelect={() => void select(role)}
            onKeyDown={onKeyDown(index)}
            cardRef={(el) => {
              cards.current[index] = el
            }}
          />
        ))}
      </div>
      {refused && (
        <p className="field-error" role="alert">
          {refused.title}: {refused.message}
        </p>
      )}
      {selected && catalogue && <Selection role={selected} catalogue={catalogue} />}
      <p className="hint">
        Memory and CPU figures are the measured floor per role, without load.{' '}
        <a href={SIZING_URL} target="_blank" rel="noreferrer">
          Sizing ↗
        </a>
      </p>
    </section>
  )
}
