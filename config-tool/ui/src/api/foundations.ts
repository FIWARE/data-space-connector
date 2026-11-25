/** GET /api/foundations: the cross-cutting answers of a chart version and where each lands (ADR 0020). */
import { api } from './client'

/** Answer type, mirroring `model.FoundationType`; a choice is checked against its options, every other type with a pattern. */
export type FoundationType = 'string' | 'hostname' | 'domain' | 'choice' | 'did' | 'url' | 'bool' | 'credentialTypes'

/** One element of a `credentialTypes` answer, mirroring the Go check; types are unique within the list. */
export interface CredentialType {
  type: string
  description?: string
}

/** One value a choice offers, mirroring `model.Option`; `note` is shown at the point of choice. */
export interface FoundationOption {
  value: string
  label: string
  note: string
}

/** The Stage 3 panel that asks a foundation, mirroring `model.FoundationGroup`. */
export type FoundationGroup = 'organization' | 'dataspace' | 'network' | 'platform'

/** Holds while the resolved answer `id` equals `equals`, mirroring `model.Condition`. */
export interface Condition {
  id: string
  equals: string
}

/** One chart key a foundation reaches; `component` is the path's top-level key. */
export interface TargetRef {
  path: string
  component: string
}

/** One foundation in bundle order, mirroring the server's `foundationBody`. */
export interface Foundation {
  id: string
  stage: string
  type: FoundationType
  group: FoundationGroup
  /** Template the answer is derived from; empty for an answer the user gives. */
  default: string
  /** Gates `default` on another answer; null when the default always applies. */
  defaultWhen: Condition | null
  /** Template the answer, given or derived, must start with; empty when there is none. */
  prefix: string
  /** The values a choice offers; empty for every other type. */
  options: FoundationOption[]
  label: string
  help: string
  /** Repo-relative doc path, empty when there is none. */
  doc: string
  /** Every key the answer reaches, its derived answers' keys included; sorted by path. */
  targets: TargetRef[]
  count: number
  components: string[]
  /** Feature ids; Stage 3 hides the foundation while any is off. Empty when it is always shown. */
  features: string[]
}

/** Kubernetes quantities as the chart receives them, mirroring `model.ResourceQuantities`. */
export interface ResourceQuantities {
  cpu: string
  memory: string
}

/** What a profile renders for one component, mirroring `model.ResourceSpec`. */
export interface ResourceSpec {
  requests: ResourceQuantities
  limits: ResourceQuantities
}

/** One component a resource profile sizes; `path` is its resources block. */
export interface ResourceComponent {
  id: string
  label: string
  path: string
}

/** The bundle's resources table as the server's `resourcesBody` previews it (ADR 0024). */
export interface ResourceTable {
  /** The choice foundation whose resolved value selects a profile. */
  answer: string
  /** Chart version the figures were measured on. */
  measuredOn: string
  /** Repo-relative doc path of the sizing guidance. */
  doc: string
  components: ResourceComponent[]
  /** Spec by profile, then component id; a choice value without a row (`custom`) renders nothing. */
  profiles: Record<string, Record<string, ResourceSpec>>
}

/** A component with the figures a profile gives it. */
export interface ResourceRow {
  component: ResourceComponent
  spec: ResourceSpec
}

/** The rows `profile` renders, in table order; undefined when the table has no such profile, so it sets nothing. */
export function resourceRows(table: ResourceTable, profile: string): ResourceRow[] | undefined {
  const specs = Object.hasOwn(table.profiles, profile) ? table.profiles[profile] : undefined
  if (specs === undefined) return undefined
  return table.components.flatMap((component) => {
    const spec = specs[component.id]
    return spec === undefined ? [] : [{ component, spec }]
  })
}

/** Answer of GET /api/foundations. */
export interface FoundationList {
  foundations: Foundation[]
  /** Null for a bundle that sizes nothing. */
  resources: ResourceTable | null
}

/** GET /api/foundations; an empty `version` means the chart's latest. 404 for a chart or version without a bundle. */
export function fetchFoundations(chart: string, version: string, signal?: AbortSignal): Promise<FoundationList> {
  return api<FoundationList>(`/foundations?${new URLSearchParams({ chart, version })}`, { signal })
}
