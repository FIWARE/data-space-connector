/** Target validation, mirroring `profile.Target.Validate` in Go: same fields, limits and messages. */
import { Environment, TargetType, type Target } from './answers'
import type { Issues } from './issues'
import type { AppState } from './reducer'

/** DNS-1123 label limit Kubernetes enforces on namespaces. */
export const MAX_NAMESPACE_LENGTH = 63
/** Helm's release name limit, shorter than a label so derived names still fit. */
export const MAX_RELEASE_NAME_LENGTH = 53

/** Field names, matching the JSON keys of Target. */
export const TargetField = {
  Type: 'type',
  Context: 'context',
  Namespace: 'namespace',
  ReleaseName: 'releaseName',
  Chart: 'chart',
  ChartVersion: 'chartVersion',
  Environment: 'environment',
} as const
export type TargetField = (typeof TargetField)[keyof typeof TargetField]

/** Marks an unanswered field, counted as an open question rather than an error. */
export const MSG_REQUIRED = 'is required'
const MSG_DNS1123 = "must consist of lowercase letters, digits and '-', and start and end with a letter or digit"
const MSG_DNS1035 = "must consist of lowercase letters, digits and '-', start with a letter and end with a letter or digit"

// Release names prefix Service names, which must be DNS-1035 labels (leading letter).
const DNS1123_LABEL = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/
const DNS1035_LABEL = /^[a-z]([-a-z0-9]*[a-z0-9])?$/

/** A validation failure of a single Target field. */
export interface FieldError {
  field: TargetField
  message: string
}

function checkLabel(name: string, maxLength: number, pattern: RegExp, patternMessage: string): string | undefined {
  if (name === '') return MSG_REQUIRED
  if (name.length > maxLength) return `must be at most ${maxLength} characters`
  if (!pattern.test(name)) return patternMessage
  return undefined
}

// A profile loaded later (CT-4) is untyped JSON, so the choice is checked, not just trusted.
function checkChoice(value: string, choices: Record<string, string>): string | undefined {
  const allowed = Object.values(choices)
  if (value === '') return MSG_REQUIRED
  if (!allowed.includes(value)) return `must be ${allowed.map((c) => `"${c}"`).join(' or ')}`
  return undefined
}

/** Every field error of `target`; the context only counts for the cluster target. */
export function validateTarget(target: Target): FieldError[] {
  const errors: FieldError[] = []
  const add = (field: TargetField, message: string | undefined) => message !== undefined && errors.push({ field, message })
  add(TargetField.Type, checkChoice(target.type, TargetType))
  if (target.type === TargetType.Cluster && !target.context) add(TargetField.Context, MSG_REQUIRED)
  add(TargetField.Namespace, checkLabel(target.namespace, MAX_NAMESPACE_LENGTH, DNS1123_LABEL, MSG_DNS1123))
  add(TargetField.ReleaseName, checkLabel(target.releaseName, MAX_RELEASE_NAME_LENGTH, DNS1035_LABEL, MSG_DNS1035))
  if (target.chart === '') add(TargetField.Chart, MSG_REQUIRED)
  if (target.chartVersion === '') add(TargetField.ChartVersion, MSG_REQUIRED)
  add(TargetField.Environment, checkChoice(target.environment, Environment))
  return errors
}

/** The Target stage's footer and rail counters. */
export function targetIssues(state: AppState): Issues {
  const errors = validateTarget(state.answers.target)
  const open = errors.filter((e) => e.message === MSG_REQUIRED).length
  return { warnings: 0, errors: errors.length - open, open }
}
