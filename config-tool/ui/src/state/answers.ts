/** The answer set: what the user decided, mirroring the Go `profile` package. */

/** Where the generated values go. */
export const TargetType = { File: 'file', Cluster: 'cluster' } as const
export type TargetType = (typeof TargetType)[keyof typeof TargetType]

/** Development or production defaults. */
export const Environment = { Development: 'development', Production: 'production' } as const
export type Environment = (typeof Environment)[keyof typeof Environment]

/** The chart a new configuration starts from. */
export const DEFAULT_CHART = 'data-space-connector'

/** Where and how the connector is deployed; only the kube context name is ever kept. */
export interface Target {
  type: TargetType
  context?: string
  namespace: string
  createNamespace: boolean
  releaseName: string
  chart: string
  chartVersion: string
  environment: Environment
}

/** Everything answered so far; later stages add their sections. */
export interface Answers {
  target: Target
}

/** A fresh answer set, as "New configuration" starts it. */
export function emptyAnswers(): Answers {
  return {
    target: {
      type: TargetType.File,
      namespace: '',
      createNamespace: false,
      releaseName: '',
      chart: DEFAULT_CHART,
      chartVersion: '',
      environment: Environment.Development,
    },
  }
}
