/** UI copy of the Stage 3 *Data space* panel and its Review line, apart from the components for fast refresh. */
import type { CredentialType } from '../api/foundations'
import type { TirCheck, TirOutcome } from '../state/dataspace'

export const DATASPACE_PANEL = 'Data space'

export const IMPORT_LABEL = 'Import from'
export const IMPORT_BUTTON = 'Import'
export const IMPORTING = 'Importing…'
export const IMPORT_HINT =
  'The data space’s address or its /.well-known/data-space-configuration document. Nothing is fetched until you press Import, and nothing changes until you apply it.'
export const PREVIEW_LABEL = 'Import preview'
export const NO_CHANGES = 'Every imported value equals the current answer.'
export const PROBLEMS_HEADING = 'Not imported'
export const KNOWN_LABEL = 'Also published, not asked here:'
export const UNUSED_LABEL = 'Not used:'
export const APPLY_LABEL = 'Apply'
export const DISCARD_LABEL = 'Discard'

export const CHECK_LABEL = 'Check'
export const CHECKING = 'Checking…'

export const TYPE_COLUMN = 'Type'
export const DESCRIPTION_COLUMN = 'Description'
export const ADD_TYPE = '+ add a credential type'
export const CREDENTIAL_TYPES_HINT = 'These seed the issuance page, the verifier registration and the local trusted-issuers list.'

export const TIR_REVIEW_HEADING = 'Trust Anchor reachability'
export const TIR_REVIEW_HINT = 'Information only: the check reflects one moment and never blocks generating values.yaml.'

const NOT_SET = '—'
const YES = 'yes'
const NO = 'no'

/** The icon next to an outcome; the text beside it carries the meaning, so colour is never the only cue. */
export const OUTCOME_ICONS: Record<TirOutcome, string> = { reachable: '✓', unreachable: '✗', skipped: '–' }

/** "reachable (HTTP 200)", "unreachable: <why>" or "not checked". */
export function outcomeText(check: TirCheck): string {
  switch (check.outcome) {
    case 'reachable':
      return check.detail ? `reachable (${check.detail})` : 'reachable'
    case 'unreachable':
      return check.detail ? `unreachable: ${check.detail}` : 'unreachable'
    case 'skipped':
      return 'not checked'
  }
}

/** An answer as the preview shows it: types (with any description) joined for a list, yes/no for a bool, a dash while unanswered. */
export function previewValue(value: unknown): string {
  if (value === undefined || value === null) return NOT_SET
  if (typeof value === 'boolean') return value ? YES : NO
  // Descriptions are shown because the preview counts a description-only difference as a change.
  if (Array.isArray(value)) return (value as CredentialType[]).map((t) => (t.description ? `${t.type} (${t.description})` : t.type)).join(', ')
  return typeof value === 'string' ? value : JSON.stringify(value)
}

/** The accessible names of a credential-type row's inputs and buttons; `n` counts from 1. */
export const rowLabels = (n: number) => ({
  type: `${TYPE_COLUMN} ${n}`,
  description: `${DESCRIPTION_COLUMN} ${n}`,
  up: `Move credential type ${n} up`,
  down: `Move credential type ${n} down`,
  remove: `Remove credential type ${n}`,
  row: `Credential type ${n}`,
})

// The `[i]` or `[i].type` prefix of a credentialTypes error, as `checkCredentialTypes` (and Go) word it.
const ROW_ERROR = /^\[(\d+)\](?:\.(type|description))? (.*)$/s

/** A credentialTypes error located on its row: `row` counts from 0 like the check, `part` is unset for the whole row. */
export interface RowError {
  row?: number
  part?: 'type' | 'description'
  text: string
}

/** `error` with its 0-based `[i]` prefix replaced by the row's 1-based label; a list-wide error stays as is. */
export function rowError(error: string): RowError {
  const m = ROW_ERROR.exec(error)
  if (!m) return { text: error }
  const row = Number(m[1])
  const part = m[2] as RowError['part']
  const labels = rowLabels(row + 1)
  return { row, part, text: `${part ? labels[part] : labels.row} ${m[3]}` }
}
