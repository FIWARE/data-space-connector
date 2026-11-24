/** The answer set: what the user decided, mirroring the Go `profile` package. */

/** Where the generated values go. */
export const TargetType = { File: 'file', Cluster: 'cluster' } as const
export type TargetType = (typeof TargetType)[keyof typeof TargetType]

/** Development or production defaults. */
export const Environment = { Development: 'development', Production: 'production' } as const
export type Environment = (typeof Environment)[keyof typeof Environment]

/** The chart a new configuration starts from. */
export const DEFAULT_CHART = 'data-space-connector'

/** A radio nobody has picked yet: an open question, never a hidden default. */
export const UNANSWERED = ''
export type Unanswered = typeof UNANSWERED

/** Where and how the connector is deployed; only the kube context name is ever kept, never the kubeconfig path. */
export interface Target {
  type: TargetType | Unanswered
  context?: string
  namespace: string
  createNamespace: boolean
  releaseName: string
  chart: string
  chartVersion: string
  environment: Environment | Unanswered
}

/** Who decided a feature's state; a re-applied preset only touches `preset` (ADR 0015 §7). */
export type FeatureSource = 'preset' | 'loaded' | 'user'

/** The recorded state of one catalogue feature, mirroring `profile.FeatureState`. */
export interface FeatureState {
  enabled: boolean
  source: FeatureSource
}

/** Everything answered so far; later stages add their sections. `features` is empty while no role is picked. */
export interface Answers {
  target: Target
  role?: string
  features: Record<string, FeatureState>
  /** Foundation answers by dotted id (`domain.base`); an absent id is unanswered and renders nothing. */
  values: Record<string, string>
}

/** A fresh answer set, as "New configuration" starts it. */
export function emptyAnswers(): Answers {
  return {
    target: {
      type: UNANSWERED,
      namespace: '',
      createNamespace: false,
      releaseName: '',
      chart: DEFAULT_CHART,
      chartVersion: '',
      environment: UNANSWERED,
    },
    features: {},
    values: {},
  }
}

/** State of the running tool that never goes into the answer set; `kubeconfig` is undefined until pre-filled or typed. */
export interface Session {
  kubeconfig?: string
  /** The context the user last picked, restored when a later read has it; CLEARED_CONTEXT blocks the preselection. */
  chosenContext?: string
}

/** `Session.chosenContext` once the user picked the empty option; undefined means never picked. */
export const CLEARED_CONTEXT = ''
