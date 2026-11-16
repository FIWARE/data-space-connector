/** S1 Target: where the configuration goes, which release and chart, and which defaults. */
import { useEffect, useState, type ReactNode } from 'react'
import { ApiError, HTTP_NOT_FOUND } from '../api/client'
import { fetchContexts, fetchModel, type Kubeconfig, type ModelInfo } from '../api/target'
import { releaseNotesUrl } from '../links'
import { Environment, TargetType, type Session, type Target as TargetAnswers } from '../state/answers'
import { useStore } from '../state/context'
import { MSG_REQUIRED, TargetField, validateTarget } from '../state/target'

/** Pause after the last keystroke in the kubeconfig field before the contexts are read again. */
const KUBECONFIG_DEBOUNCE_MS = 300

const TARGET_CHOICES = [
  { value: TargetType.File, title: 'File only', text: 'Generate values.yaml and install it yourself.' },
  { value: TargetType.Cluster, title: 'Kubernetes cluster', text: 'Read the cluster to pre-fill the next screens. Nothing is installed before step 8.' },
] as const

const ENVIRONMENT_CHOICES = [
  { value: Environment.Development, title: 'Development', text: 'Self-signed certificates, did:key, chart-deployed databases, no resource limits.' },
  { value: Environment.Production, title: 'Production', text: 'ACME or your own CA, did:web, resource limits, replicas.' },
] as const

function useDebounced<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])
  return debounced
}

function useModel(): { model?: ModelInfo; error?: string } {
  const [result, setResult] = useState<{ model?: ModelInfo; error?: string }>({})
  useEffect(() => {
    const controller = new AbortController()
    fetchModel(controller.signal).then(
      (model) => setResult({ model }),
      (err: Error) => controller.signal.aborted || setResult({ error: err.message }),
    )
    return () => controller.abort()
  }, [])
  return result
}

interface ContextsResult {
  kubeconfig?: Kubeconfig
  notFound?: boolean
  error?: string
}

function useContexts(path: string): ContextsResult {
  const [result, setResult] = useState<ContextsResult>({})
  useEffect(() => {
    const controller = new AbortController()
    fetchContexts(path, controller.signal).then(
      (kubeconfig) => setResult({ kubeconfig }),
      (err: Error) =>
        controller.signal.aborted || setResult({ notFound: err instanceof ApiError && err.status === HTTP_NOT_FOUND, error: err.message }),
    )
    return () => controller.abort()
  }, [path])
  return result
}

function Choices<T extends string>({
  name,
  legend,
  choices,
  value,
  onChange,
}: {
  name: string
  legend: string
  choices: readonly { value: T; title: string; text: string }[]
  value: string
  onChange: (value: T) => void
}) {
  return (
    <fieldset className="modes">
      <legend>{legend}</legend>
      {choices.map((choice) => (
        <label key={choice.value} className="mode">
          <input type="radio" name={name} value={choice.value} checked={value === choice.value} onChange={() => onChange(choice.value)} />
          <span className="txt">
            <b>{choice.title}</b>
            <span>{choice.text}</span>
          </span>
        </label>
      ))}
    </fieldset>
  )
}

function Row({ id, label, children, error }: { id: string; label: string; children: ReactNode; error?: string }) {
  return (
    <div className="row">
      <label htmlFor={id}>{label}</label>
      {children}
      {error && (
        <p className="field-error" id={`${id}-error`}>
          {error}
        </p>
      )}
    </div>
  )
}

/** Kubeconfig path and context; mounted only for the cluster target, so nothing is read for "File only". */
function ClusterFields({ target, session }: { target: TargetAnswers; session: Session }) {
  const { dispatch } = useStore()
  const path = session.kubeconfig ?? ''
  const { kubeconfig, notFound, error } = useContexts(useDebounced(path, KUBECONFIG_DEBOUNCE_MS))

  useEffect(() => {
    if (!kubeconfig) return
    if (session.kubeconfig === undefined) {
      const [only] = kubeconfig.paths
      dispatch({ type: 'setSession', session: { kubeconfig: kubeconfig.paths.length === 1 && only ? only : '' } })
    }
    if (target.context && kubeconfig.contexts.includes(target.context)) return
    const current = kubeconfig.contexts.includes(kubeconfig.current) ? kubeconfig.current : undefined
    if (current !== target.context) dispatch({ type: 'setTarget', target: { context: current } })
  }, [kubeconfig, session.kubeconfig, target.context, dispatch])

  const several = path === '' && kubeconfig && kubeconfig.paths.length > 1
  const contexts = kubeconfig?.contexts ?? (target.context ? [target.context] : [])
  return (
    <div className="cluster">
      <Row id="kubeconfig" label="Kubeconfig" error={notFound ? `No kubeconfig found at ${path}.` : error}>
        <input
          id="kubeconfig"
          type="text"
          value={path}
          placeholder="default loading rules"
          aria-describedby={notFound || error ? 'kubeconfig-error' : undefined}
          onChange={(e) => dispatch({ type: 'setSession', session: { kubeconfig: e.target.value } })}
        />
      </Row>
      {several && <p className="hint">Read from KUBECONFIG, in this order: {kubeconfig.paths.join(', ')}</p>}
      <Row id="context" label="Context">
        <select id="context" value={target.context ?? ''} onChange={(e) => dispatch({ type: 'setTarget', target: { context: e.target.value || undefined } })}>
          <option value="">— choose a context —</option>
          {contexts.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </Row>
      <p className="hint">The cluster is only read, to pre-fill the next screens. Nothing is installed before you ask for it on step 8.</p>
    </div>
  )
}

/** S1 Target screen. */
export function Target() {
  const { state, dispatch } = useStore()
  const target = state.answers.target
  const set = (change: Partial<TargetAnswers>) => dispatch({ type: 'setTarget', target: change })
  const { model, error: modelError } = useModel()
  const cluster = target.type === TargetType.Cluster

  useEffect(() => {
    const latest = model?.versions.find((v) => v.latest)
    if (latest && target.chartVersion === '') dispatch({ type: 'setTarget', target: { chartVersion: latest.version } })
  }, [model, target.chartVersion, dispatch])

  const errors = validateTarget(target)
  const errorOf = (field: TargetField) => errors.find((e) => e.field === field && e.message !== MSG_REQUIRED)?.message
  const charts = model?.charts ?? [target.chart]
  // Until the model arrives, the chosen version stays selectable so a revisit does not blank it.
  const versions = model?.versions ?? (target.chartVersion ? [{ version: target.chartVersion, latest: false }] : [])

  return (
    <section>
      <h1>Where is this going?</h1>
      <p className="lede">The target decides what can be detected and what has to be typed.</p>

      <div className="panel">
        <Choices name="target" legend="Target" choices={TARGET_CHOICES} value={target.type} onChange={(type) => set({ type })} />
        {cluster && <ClusterFields target={target} session={state.session} />}
      </div>

      <div className="panel">
        <h3>Release</h3>
        <Row id="namespace" label="Namespace" error={errorOf(TargetField.Namespace)}>
          <input id="namespace" type="text" value={target.namespace} onChange={(e) => set({ namespace: e.target.value })} />
          {cluster && (
            <label className="inline">
              <input type="checkbox" checked={target.createNamespace} onChange={(e) => set({ createNamespace: e.target.checked })} /> create it
            </label>
          )}
        </Row>
        <Row id="release-name" label="Release name" error={errorOf(TargetField.ReleaseName)}>
          <input id="release-name" type="text" value={target.releaseName} onChange={(e) => set({ releaseName: e.target.value })} />
        </Row>
        <p className="hint">ⓘ Both appear in generated service URLs and in the Trust Anchor registration.</p>
      </div>

      <div className="panel">
        <h3>Chart</h3>
        {modelError && <p className="field-error">{modelError}</p>}
        <Row id="chart" label="Chart">
          <select id="chart" value={target.chart} onChange={(e) => set({ chart: e.target.value })}>
            {charts.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </Row>
        <Row id="chart-version" label="Version">
          <select id="chart-version" value={target.chartVersion} onChange={(e) => set({ chartVersion: e.target.value })}>
            {target.chartVersion === '' && <option value="">— choose a version —</option>}
            {versions.map((v) => (
              <option key={v.version} value={v.version}>
                {v.latest ? `${v.version} (latest)` : v.version}
              </option>
            ))}
          </select>
          {target.chartVersion && (
            <a href={releaseNotesUrl(target.chartVersion)} target="_blank" rel="noreferrer">
              Release notes ↗
            </a>
          )}
        </Row>
      </div>

      <div className="panel">
        <Choices
          name="environment"
          legend="Environment"
          choices={ENVIRONMENT_CHOICES}
          value={target.environment}
          onChange={(environment) => set({ environment })}
        />
        <p className="hint">ⓘ Only changes defaults. Every one of them stays editable.</p>
      </div>
    </section>
  )
}
