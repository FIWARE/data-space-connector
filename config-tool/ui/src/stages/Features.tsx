/** S5 Features: one toggle per feature of the target chart, starting from the role preset (ADR 0018). */
import { useEffect, useRef, useState } from 'react'
import { fetchFeatures, type FeatureChoice, type FeatureToggle } from '../api/features'
import { toProfile, type FeatureState, type Profile } from '../api/profile'
import type { Feature, Offer, Role as RoleDef } from '../api/roles'
import { ConfirmDialog } from '../chrome/ConfirmDialog'
import { repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import type { AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { DerivedAnswers } from './DerivedAnswers'
import { useRoles } from './useRoles'

/** Heading of the stage. */
export const FEATURES_HEADING = 'Which features should this connector run?'
/** Lede under the heading: the preset is where the toggles start, not a limit. */
export const FEATURES_LEDE = 'Your role decides where each feature starts. Every toggle stays yours to change.'
/** Hint shown before a role is picked, followed by a link to step 2. */
export const NO_ROLE_HINT = 'Features start from a role preset. Pick a role on'
/** Hint under the loaded file's answers while no role is picked; with a role the toggles show them instead. */
export const NO_ROLE_ANSWERS_HINT = 'Shown read-only until a role is picked. The choices of your values file then show up in the toggles.'
/** Text of the link to step 2 in the hint. */
export const NO_ROLE_LINK = 'step 2'
/** Heading of the section of features the role does not preselect but offers. */
export const AVAILABLE_SECTION = 'Available'
/** Heading of the section of features the role does not offer. */
export const NOT_RECOMMENDED_SECTION = 'Not recommended for this role'
/** The confirm dialog's default answer. */
export const KEEP_ON = 'Keep it on'
/** The confirm dialog's answer that turns the feature off. */
export const TURN_OFF = 'Turn it off'

const SECTIONS: readonly { offer: Offer; title: (role: RoleDef) => string }[] = [
  { offer: 'recommended', title: (role) => `Recommended for ${role.title}` },
  { offer: 'available', title: () => AVAILABLE_SECTION },
  { offer: 'not-offered', title: () => NOT_RECOMMENDED_SECTION },
]

const PRESET_WORDS: Record<Offer, string> = {
  recommended: 'recommended on for',
  available: 'off by default for',
  'not-offered': 'not recommended for',
}

/** Why a feature differs from the preset: what the role wants, then who decided otherwise. */
function deliberateMarker(choice: FeatureChoice, role: RoleDef): string {
  const state = choice.enabled ? 'on' : 'off'
  const who = choice.source === 'loaded' ? `your values file turns it ${state}` : `you turned it ${state}`
  return `${PRESET_WORDS[choice.offer]} ${role.title} — ${who}`
}

function withoutKey(errors: Record<string, string>, key: string): Record<string, string> {
  return Object.fromEntries(Object.entries(errors).filter(([id]) => id !== key))
}

/** One checkbox; never disabled, so a preset locks nothing. */
function FeatureRow({
  feature,
  choice,
  role,
  error,
  onToggle,
}: {
  feature: Feature
  choice: FeatureChoice
  role: RoleDef
  error?: string
  onToggle: () => void
}) {
  const id = `feature-${feature.id}`
  return (
    <div>
      <label className="mode">
        <input
          type="checkbox"
          checked={choice.enabled}
          onChange={onToggle}
          aria-labelledby={`${id}-title`}
          aria-describedby={choice.deliberate ? `${id}-summary ${id}-marker` : `${id}-summary`}
        />
        <span className="txt">
          <strong id={`${id}-title`}>{feature.title}</strong>
          <span id={`${id}-summary`}>{feature.summary}</span>
          {choice.deliberate && (
            <span className="deliberate" id={`${id}-marker`}>
              {deliberateMarker(choice, role)}
            </span>
          )}
        </span>
      </label>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

function profileOf(state: AppState, features: Record<string, FeatureState>): Profile {
  return { ...toProfile(state), features }
}

/** The toggle list once a role is picked. */
function FeatureList({ roleId }: { roleId: string }) {
  const { state, dispatch } = useStore()
  const { chart, chartVersion } = state.answers.target
  const { catalogue, error: rolesError } = useRoles(chart, chartVersion)
  const [choices, setChoices] = useState<FeatureChoice[]>()
  const [loadError, setLoadError] = useState<string>()
  const [refused, setRefused] = useState<Record<string, string>>({})
  const [queued, setQueued] = useState(0)
  const [confirming, setConfirming] = useState<Feature>()
  const latest = useRef(state)
  // Each request carries the whole profile, so the next one starts from the previous response, not from the render.
  const features = useRef(state.answers.features)
  const queue = useRef(Promise.resolve())
  const depth = useRef(0)
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  useEffect(() => {
    latest.current = state
    // While toggles are queued the last response is ahead of the store.
    if (depth.current === 0) features.current = state.answers.features
  }, [state])

  useEffect(() => {
    fetchFeatures(profileOf(latest.current, features.current), latest.current.loaded?.sha256).then(
      (result) => mounted.current && setChoices(result.choices),
      (err: Error) => mounted.current && setLoadError(err.message),
    )
  }, [])

  const send = (toggle: FeatureToggle) => {
    depth.current += 1
    setQueued((n) => n + 1)
    setRefused((errors) => withoutKey(errors, toggle.feature))
    queue.current = queue.current.then(async () => {
      // A response after the stage was left must not change a configuration started since; the rest of the queue ends.
      if (!mounted.current) return
      try {
        const result = await fetchFeatures(profileOf(latest.current, features.current), latest.current.loaded?.sha256, toggle)
        if (!mounted.current) return
        features.current = result.profile.features ?? {}
        setChoices(result.choices)
        dispatch({ type: 'featuresChanged', features: features.current })
      } catch (err) {
        if (mounted.current) setRefused((errors) => ({ ...errors, [toggle.feature]: (err as Error).message }))
      } finally {
        depth.current -= 1
        if (mounted.current) setQueued((n) => n - 1)
      }
    })
  }

  const role = catalogue?.roles.find((r) => r.id === roleId)
  const error = rolesError ?? loadError
  if (error) return <p className="field-error">{error}</p>
  if (!role || !catalogue || !choices) return null

  const toggle = (feature: Feature, choice: FeatureChoice) => {
    if (choice.enabled && choice.offer === 'recommended') setConfirming(feature)
    else send({ feature: feature.id, enabled: !choice.enabled })
  }
  const titled = choices.flatMap((choice) => {
    const feature = catalogue.features.find((f) => f.id === choice.id)
    return feature ? [{ feature, choice }] : []
  })

  return (
    <div role="group" aria-label="Features" aria-busy={queued > 0}>
      {SECTIONS.map(({ offer, title }) => {
        const rows = titled.filter(({ choice }) => choice.offer === offer)
        if (rows.length === 0) return null
        const headingId = `features-${offer}`
        return (
          <div key={offer} className="panel" role="group" aria-labelledby={headingId}>
            <h3 id={headingId}>{title(role)}</h3>
            <div className="modes">
              {rows.map(({ feature, choice }) => (
                <FeatureRow
                  key={feature.id}
                  feature={feature}
                  choice={choice}
                  role={role}
                  error={refused[feature.id]}
                  onToggle={() => toggle(feature, choice)}
                />
              ))}
            </div>
          </div>
        )
      })}
      {confirming && (
        <ConfirmDialog
          title={`Turn off ${confirming.title}?`}
          cancelLabel={KEEP_ON}
          confirmLabel={TURN_OFF}
          onCancel={() => setConfirming(undefined)}
          onConfirm={() => {
            setConfirming(undefined)
            send({ feature: confirming.id, enabled: false })
          }}
        >
          <p>{confirming.consequence || confirming.summary}</p>
          <p className="hint">
            <a href={repositoryDocUrl(role.docs)} target="_blank" rel="noreferrer">
              What the {role.title} role needs ↗
            </a>
          </p>
        </ConfirmDialog>
      )}
    </div>
  )
}

/** S5 Features screen. */
export function Features() {
  const { state, dispatch } = useStore()
  const roleId = state.answers.role
  return (
    <section>
      <h1>{FEATURES_HEADING}</h1>
      <p className="lede">{FEATURES_LEDE}</p>
      {roleId ? (
        <FeatureList roleId={roleId} />
      ) : (
        <>
          <p className="hint">
            {NO_ROLE_HINT}{' '}
            <button type="button" className="link" onClick={() => dispatch({ type: 'goTo', stage: StageId.Role })}>
              {NO_ROLE_LINK}
            </button>
            .
          </p>
          <DerivedAnswers stage={StageId.Features} hint={NO_ROLE_ANSWERS_HINT} />
        </>
      )}
    </section>
  )
}
