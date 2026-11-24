/** UI copy of the L2 findings on S3 Foundations and S7 Review, apart from the components so fast refresh keeps working. */
import type { Severity } from '../api/render'
import { STAGE_LABELS, StageId } from '../state/stageIds'

/** Heading of Review's section. */
export const FINDINGS_HEADING = 'Findings'
/** Why a warning does not stop the user. */
export const FINDINGS_HINT = 'Warnings never block generating values.yaml; only errors do.'
/** The link to a finding's doc. */
export const READ_MORE = 'Read more ↗'
/** How a finding names its severity. */
export const SEVERITY_LABELS: Record<Severity, string> = { error: 'Error', warning: 'Warning' }

/** The number the rail shows for the first stage. */
const FIRST_STAGE_NUMBER = 1

/** "Stage 3 · Foundations" for the foundations stage, numbered in rail order. */
export function stageName(stage: StageId): string {
  return `Stage ${Object.values(StageId).indexOf(stage) + FIRST_STAGE_NUMBER} · ${STAGE_LABELS[stage]}`
}
