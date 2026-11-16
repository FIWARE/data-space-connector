/** S1 Target: where the configuration goes, which release and chart, and which defaults. */
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { fetchContexts, fetchModel, type Kubeconfig, type ModelInfo } from '../api/target'
import { releaseNotesUrl } from '../links'
import { CLEARED_CONTEXT, Environment, TargetType, type Session, type Target as TargetAnswers } from '../state/answers'
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

/** Outcome of reading the contexts of `path`; a later path replaces it. */
interface ContextsResult {
  path: string
  kubeconfig?: Kubeconfig
  error?: string
}

function useContexts(path: string): ContextsResult | undefined {
  const [result, setResult] = useState<ContextsResult>()
  useEffect(() => {
    const controller = new AbortController()
    fetchContexts(path, controller.signal).then(
      (kubeconfig) => setResult({ path, kubeconfig }),
      (err: Error) => controller.signal.aborted || setResult({ path, error: err.message }),
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
  const result = useContexts(useDebounced(path, KUBECONFIG_DEBOUNCE_MS))
  // Only a result for the path in the field counts (the prefill swaps '' for the one default file
  // read); while the next one is read, nothing is reported.
  const read = result?.kubeconfig?.paths
  const checked = result && (result.path === path || (read?.length === 1 && read[0] === path)) ? result : undefined
  const kubeconfig = checked?.kubeconfig
  const reconciled = useRef<ContextsResult>()

  useEffect(() => {
    if (result?.kubeconfig && session.kubeconfig === undefined) {
      const [only] = result.kubeconfig.paths
      dispatch({ type: 'setSession', session: { kubeconfig: result.kubeconfig.paths.length === 1 && only ? only : '' } })
    }
  }, [result, session.kubeconfig, dispatch])

  // Once per read: keep a context the file still has, else restore the user's last pick, else
  // preselect the file's current one unless the user cleared the pick. A failed read clears it, so
  // the footer counts it open, but the pick survives in the session for when the path is corrected.
  useEffect(() => {
    if (!checked || reconciled.current === checked) return
    reconciled.current = checked
    const names = checked.kubeconfig?.contexts ?? []
    if (target.context && names.includes(target.context)) return
    const fallback = session.chosenContext === CLEARED_CONTEXT ? undefined : checked.kubeconfig?.current
    const preferred = [session.chosenContext, fallback].find((name) => name && names.includes(name))
    if (preferred !== target.context) dispatch({ type: 'setTarget', target: { context: preferred } })
  }, [checked, target.context, session.chosenContext, dispatch])

  const choose = (name: string) => {
    dispatch({ type: 'setTarget', target: { context: name || undefined } })
    dispatch({ type: 'setSession', session: { chosenContext: name } })
  }

  const several = path === '' && kubeconfig && kubeconfig.paths.length > 1
  const noneFound = path === '' && kubeconfig?.paths.length === 0
  // A relative or ~ path is resolved by the server; show which file that turned out to be.
  const [resolved] = path !== '' && kubeconfig?.paths.length === 1 ? kubeconfig.paths : []
  const resolvedElsewhere = resolved !== undefined && resolved !== path ? resolved : undefined
  // The server's message names the file and the cause: missing, not a regular file, or denied.
  const error = checked?.error
  // While a path is read, the chosen context stays selectable so a revisit does not blank it.
  const contexts = checked ? (kubeconfig?.contexts ?? []) : target.context ? [target.context] : []
  return (
    <div className="cluster">
      <Row id="kubeconfig" label="Kubeconfig" error={error}>
        <input
          id="kubeconfig"
          type="text"
          value={path}
          placeholder="default loading rules"
          aria-describedby={error ? 'kubeconfig-error' : undefined}
          onChange={(e) => dispatch({ type: 'setSession', session: { kubeconfig: e.target.value } })}
        />
      </Row>
      {several && <p className="hint">Read from KUBECONFIG, in this order: {kubeconfig.paths.join(', ')}</p>}
      {noneFound && <p className="hint">No kubeconfig found (KUBECONFIG or ~/.kube/config). Type the path to one.</p>}
      {resolvedElsewhere && <p className="hint">Read from {resolvedElsewhere}</p>}
      <Row id="context" label="Context">
        <select id="context" value={target.context ?? ''} onChange={(e) => choose(e.target.value)}>
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

  const chartVersions = model?.charts.find((c) => c.name === target.chart)?.versions

  useEffect(() => {
    const latest = chartVersions?.find((v) => v.latest)
    if (latest && target.chartVersion === '') dispatch({ type: 'setTarget', target: { chartVersion: latest.version } })
  }, [chartVersions, target.chartVersion, dispatch])

  const errors = validateTarget(target)
  const errorOf = (field: TargetField) => errors.find((e) => e.field === field && e.message !== MSG_REQUIRED)?.message
  const charts = model?.charts.map((c) => c.name) ?? [target.chart]
  // Until the model arrives, the chosen version stays selectable so a revisit does not blank it.
  const versions = model ? (chartVersions ?? []) : target.chartVersion ? [{ version: target.chartVersion, latest: false }] : []

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
          <select id="chart" value={target.chart} onChange={(e) => set({ chart: e.target.value, chartVersion: '' })}>
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
            <a href={releaseNotesUrl(target.chart, target.chartVersion)} target="_blank" rel="noreferrer">
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
