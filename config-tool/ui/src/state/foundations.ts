/** Foundation answer validation, mirroring `model.CompiledFoundation.Check` in Go: same patterns, limit and messages. */
import type { Foundation, FoundationOption, FoundationType } from '../api/foundations'

/** DNS name limit, `maxHostnameLength` in Go. */
export const MAX_HOSTNAME_LENGTH = 253

export const MSG_PLAIN_LINE = `must be a single non-empty line without leading or trailing spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_HOSTNAME = `must be a host name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_DOMAIN = 'must be a domain of at least two labels'
export const MSG_DID =
  "must be a DID: did:<method>:<identifier>, the method of lowercase letters and digits, the identifier of letters, digits, '.', '_', '%', ':' and '-'"

/** The choice error, listing the offered values as Go does. */
export function msgChoice(options: readonly FoundationOption[]): string {
  return `must be one of ${options.map((o) => o.value).join(', ')}`
}

// Go's \s is ASCII-only, so it is spelled out rather than taking JavaScript's Unicode \s.
// eslint-disable-next-line no-control-regex -- mirrors model.plainLine in Go
const PLAIN_LINE = /^[^\t\n\f\r \x00-\x1f\x7f"\\<>]([^\x00-\x1f\x7f"\\<>]*[^\t\n\f\r \x00-\x1f\x7f"\\<>])?$/u
const HOSTNAME = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/
const DID = /^did:[a-z0-9]+:[A-Za-z0-9._%:-]+$/

/** Why `value` is not a valid answer of `type`; undefined when it is. `options` are a choice's offered values. */
export function checkAnswer(type: FoundationType, value: string, options: readonly FoundationOption[] = []): string | undefined {
  if (type === 'string') return PLAIN_LINE.test(value) ? undefined : MSG_PLAIN_LINE
  if (type === 'choice') return options.some((o) => o.value === value) ? undefined : msgChoice(options)
  if (type === 'did') return DID.test(value) ? undefined : MSG_DID
  if (value.length > MAX_HOSTNAME_LENGTH || !HOSTNAME.test(value)) return MSG_HOSTNAME
  if (type === 'domain' && !value.includes('.')) return MSG_DOMAIN
  return undefined
}

/** The format error of every given answer, by answer id; an id the foundations do not declare is not checked. */
export function answerErrors(values: Record<string, string>, foundations: readonly Foundation[] = []): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const f of foundations) {
    const value = values[f.id]
    const error = value === undefined ? undefined : checkAnswer(f.type, value, f.options)
    if (error !== undefined) errors[f.id] = error
  }
  return errors
}

/** The answers the server accepts: an answer with a format error stays on screen but is not sent. */
export function validAnswers(values: Record<string, string>, foundations?: readonly Foundation[]): Record<string, string> {
  const errors = answerErrors(values, foundations)
  return Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
}
