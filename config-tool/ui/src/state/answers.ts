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

/** Everything answered so far; later stages add their sections. */
export interface Answers {
  target: Target
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
