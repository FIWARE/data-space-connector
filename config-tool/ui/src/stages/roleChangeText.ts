/** UI copy of the role-change summary and its dialog (ADR 0019); the core returns ids and booleans only. */
import type { FeatureSource } from '../api/profile'
import type { ChartSwitch } from '../api/roles'

/** Section headings of the summary, in display order. */
export const TURNS_ON = 'Turns on'
export const TURNS_OFF = 'Turns off'
export const KEPT = 'Kept as they are'
export const CARRIED_OVER = 'Carried over'
export const DROPPED = 'Dropped'
/** Text of a change that touches no feature. */
export const NO_FEATURE_CHANGES = 'No feature changes.'
/** Why a kept toggle keeps its state, by who set it. */
export const KEPT_REASON: Readonly<Record<FeatureSource, string>> = {
  user: 'you set it',
  loaded: 'from your values.yaml',
  preset: 'from the role',
}
/** Target fields and marks a chart switch keeps. */
export const CARRIED_FIELDS: readonly string[] = ['Namespace', 'Release name', 'Target type', 'Kube context', 'Environment', 'Open-question marks']

/** The leading sentence of a chart switch. */
export function chartMoveSentence(sw: ChartSwitch): string {
  return `The whole configuration moves from the ${sw.fromChart} chart to the ${sw.toChart} chart.`
}

/** The chart version a switch drops, as listed under Dropped. */
export function droppedVersion(sw: ChartSwitch): string {
  return sw.fromVersion ? `Chart version ${sw.fromVersion}` : 'Chart version'
}

/** The Dropped row that stands for every feature choice of the old chart. */
export function droppedFeatureChoices(sw: ChartSwitch): string {
  return `The feature choices of the ${sw.fromChart} chart`
}

/** Dialog title. */
export const changeTitle = (from: string, to: string) => `Change the role from ${from} to ${to}?`
/** Cancel label: keep the old role. */
export const keepLabel = (from: string) => `Keep ${from}`
/** Confirm label of a change within the chart. */
export const changeLabel = (to: string) => `Change to ${to}`
/** Confirm label of a change that switches the chart. */
export const moveLabel = (chart: string) => `Move to ${chart}`
