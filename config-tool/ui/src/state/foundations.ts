/** Foundation answer validation and resolution, mirroring `model.CompiledFoundation.Check` and `render.resolve` in Go. */
import type { Foundation, FoundationOption, FoundationType } from '../api/foundations'

/** DNS name limit, `maxHostnameLength` in Go. */
export const MAX_HOSTNAME_LENGTH = 253

export const MSG_PLAIN_LINE = `must be a single non-empty line without leading or trailing spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_HOSTNAME = `must be a host name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_DOMAIN = 'must be a domain of at least two labels'
export const MSG_DID =
  "must be a DID: did:<method>:<identifier>, the method of lowercase letters and digits, the identifier of letters, digits, '.', '_', '-', %XX escapes and ':', not ending in ':'"

/** The error of an answer that does not start with its prefix, which other answers set. */
export function msgPrefix(prefix: string): string {
  return `must start with "${prefix}"`
}

/** The choice error, listing the offered values as Go does. */
export function msgChoice(options: readonly FoundationOption[]): string {
  return `must be one of ${options.map((o) => o.value).join(', ')}`
}

// Go's \s is ASCII-only, so it is spelled out rather than taking JavaScript's Unicode \s.
// eslint-disable-next-line no-control-regex -- mirrors model.plainLine in Go
const PLAIN_LINE = /^[^\t\n\f\r \x00-\x1f\x7f"\\<>]([^\x00-\x1f\x7f"\\<>]*[^\t\n\f\r \x00-\x1f\x7f"\\<>])?$/u
const HOSTNAME = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/
// DID Core's idchar, ALPHA / DIGIT / "." / "-" / "_" / pct-encoded, as `didIDChar` in Go.
const DID_ID_CHAR = '(?:[A-Za-z0-9._-]|%[0-9A-Fa-f]{2})'
const DID = new RegExp(`^did:[a-z0-9]+:(?:${DID_ID_CHAR}*:)*${DID_ID_CHAR}+$`)

/** Why `value` is not a valid answer of `type`; undefined when it is. `options` are a choice's offered values. */
export function checkAnswer(type: FoundationType, value: string, options: readonly FoundationOption[] = []): string | undefined {
  if (type === 'string') return PLAIN_LINE.test(value) ? undefined : MSG_PLAIN_LINE
  if (type === 'choice') return options.some((o) => o.value === value) ? undefined : msgChoice(options)
  if (type === 'did') return DID.test(value) ? undefined : MSG_DID
  if (value.length > MAX_HOSTNAME_LENGTH || !HOSTNAME.test(value)) return MSG_HOSTNAME
  if (type === 'domain' && !value.includes('.')) return MSG_DOMAIN
  return undefined
}

/** A template's `{{ .id }}` references, the only action the bundles may use. */
const ANSWER_REFERENCE = /\{\{\s*\.([\w.]+)\s*\}\}/g

/** `template` with its references filled from resolved answers; undefined while one of them is unresolved. */
function fill(template: string, values: Record<string, string>, foundations: readonly Foundation[]): string | undefined {
  let missing = false
  const filled = template.replace(ANSWER_REFERENCE, (_, id: string) => {
    const value = resolvedAnswer(id, values, foundations)
    missing ||= value === undefined
    return value ?? ''
  })
  return missing ? undefined : filled
}

/** Whether `f`'s default applies: it has one and its `defaultWhen` holds over the resolved answers. */
export function defaultApplies(f: Foundation, values: Record<string, string>, foundations: readonly Foundation[]): boolean {
  if (f.default === '') return false
  return !f.defaultWhen || resolvedAnswer(f.defaultWhen.id, values, foundations) === f.defaultWhen.equals
}

/** `f`'s default with its references filled; undefined when it does not apply or a reference is unresolved. */
export function defaultValue(f: Foundation, values: Record<string, string>, foundations: readonly Foundation[]): string | undefined {
  return defaultApplies(f, values, foundations) ? fill(f.default, values, foundations) : undefined
}

/** The answer `id` resolves to: the given one, else its applicable default; undefined while neither exists. */
export function resolvedAnswer(id: string, values: Record<string, string>, foundations: readonly Foundation[]): string | undefined {
  if (values[id] !== undefined) return values[id]
  const f = foundations.find((o) => o.id === id)
  return f && defaultValue(f, values, foundations)
}

/** Why the given `value` of `f` does not start with its prefix; undefined when it does or the prefix is unresolved. */
function prefixError(f: Foundation, value: string, values: Record<string, string>, foundations: readonly Foundation[]): string | undefined {
  if (!f.prefix) return undefined
  const prefix = fill(f.prefix, values, foundations)
  return prefix === undefined || value.startsWith(prefix) ? undefined : msgPrefix(prefix)
}

/** The error of every given answer, by answer id: its format, then its prefix over the well-formed answers. An id the
 *  foundations do not declare is not checked. */
export function answerErrors(values: Record<string, string>, foundations: readonly Foundation[] = []): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const f of foundations) {
    const value = values[f.id]
    const error = value === undefined ? undefined : checkAnswer(f.type, value, f.options)
    if (error !== undefined) errors[f.id] = error
  }
  const wellFormed = Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
  for (const f of foundations) {
    const value = wellFormed[f.id]
    const error = value === undefined ? undefined : prefixError(f, value, wellFormed, foundations)
    if (error !== undefined) errors[f.id] = error
  }
  return errors
}

/** `values` without the answers whose prefix references `changed` and no longer matches, so a derived default can take
 *  over again; the changed answer itself is kept. */
export function dropMismatched(values: Record<string, string>, changed: string, foundations: readonly Foundation[] = []): Record<string, string> {
  const stale = foundations.filter((f) => {
    const value = values[f.id]
    if (f.id === changed || value === undefined) return false
    return [...f.prefix.matchAll(ANSWER_REFERENCE)].some(([, id]) => id === changed) && prefixError(f, value, values, foundations) !== undefined
  })
  if (stale.length === 0) return values
  return Object.fromEntries(Object.entries(values).filter(([id]) => !stale.some((f) => f.id === id)))
}

/** The answers the server accepts: an answer with a format error stays on screen but is not sent. */
export function validAnswers(values: Record<string, string>, foundations?: readonly Foundation[]): Record<string, string> {
  const errors = answerErrors(values, foundations)
  return Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
}
