/** Foundation answer validation and resolution, mirroring `model.CompiledFoundation.Check` and `render.resolve` in Go. */
import type { CredentialType, Foundation, FoundationOption, FoundationType } from '../api/foundations'
import type { AnswerValue } from './answers'

/** DNS name limit, `maxHostnameLength` in Go. */
export const MAX_HOSTNAME_LENGTH = 253
/** Longest url answer, `maxURLLength` in Go. */
export const MAX_URL_LENGTH = 2048
/** Longest credentialTypes answer, `maxCredentialTypes` in Go. */
export const MAX_CREDENTIAL_TYPES = 64

export const MSG_PLAIN_LINE = `must be a single non-empty line without leading or trailing spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_HOSTNAME = `must be a host name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_DOMAIN = 'must be a domain of at least two labels'
export const MSG_STRING = 'must be a string'
export const MSG_URL =
  `must be an absolute http or https URL with a host, without user info, fragment, spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_URL_LENGTH = `must be a URL of at most ${MAX_URL_LENGTH} characters`
export const MSG_BOOL = 'must be true or false'
export const MSG_CREDENTIAL_TYPES = 'must be a non-empty list of credential types'
export const MSG_CREDENTIAL_TYPES_LENGTH = `must list at most ${MAX_CREDENTIAL_TYPES} credential types`
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
// Go's url.Parse reads "https:host" as opaque, without a host, so the authority's "//" is required here too.
const URL_PREFIX = /^https?:\/\//i
// Go's url.Parse takes only these in a host and port; WHATWG URL also takes '{', '}', '|', '^' and '`'.
const URL_HOST = /^[A-Za-z0-9\-._~!$&'()*+,;=[\]:%\u{80}-\u{10ffff}]*$/u
// Go refuses a host's %XX escape of an ASCII byte other than %25, which WHATWG URL decodes.
const URL_HOST_ASCII_ESCAPE = /%(?!25)[0-7]/
const CREDENTIAL_TYPE = /^[A-Za-z][A-Za-z0-9._-]*$/
const CREDENTIAL_TYPE_KEYS = ['type', 'description']

function checkURL(value: string): string | undefined {
  // Go's len counts UTF-8 bytes, not UTF-16 units.
  if (new TextEncoder().encode(value).length > MAX_URL_LENGTH) return MSG_URL_LENGTH
  if (!PLAIN_LINE.test(value) || /\s|#/u.test(value) || !URL_PREFIX.test(value)) return MSG_URL
  // Go refuses any userinfo, even an empty one, which WHATWG URL parsing would hide.
  const authority = value.replace(URL_PREFIX, '').split(/[/?]/, 1)[0] ?? ''
  if (authority.includes('@') || !URL_HOST.test(authority) || URL_HOST_ASCII_ESCAPE.test(authority)) return MSG_URL
  try {
    return new URL(value).hostname === '' ? MSG_URL : undefined
  } catch {
    return MSG_URL
  }
}

function checkCredentialTypes(value: AnswerValue): string | undefined {
  if (!Array.isArray(value) || value.length === 0) return MSG_CREDENTIAL_TYPES
  if (value.length > MAX_CREDENTIAL_TYPES) return MSG_CREDENTIAL_TYPES_LENGTH
  const seen = new Set<string>()
  for (const [i, e] of (value as unknown[]).entries()) {
    if (typeof e !== 'object' || e === null || Array.isArray(e)) return `[${i}] must be a mapping with type and an optional description`
    if (Object.keys(e).some((k) => !CREDENTIAL_TYPE_KEYS.includes(k))) return `[${i}] has a key other than type and description`
    const { type, description } = e as Partial<CredentialType>
    if (typeof type !== 'string' || !CREDENTIAL_TYPE.test(type)) {
      return `[${i}].type must start with a letter, followed by letters, digits, '.', '_' and '-'`
    }
    if (seen.has(type)) return `[${i}].type repeats an earlier type`
    seen.add(type)
    if (description !== undefined && (typeof description !== 'string' || !PLAIN_LINE.test(description))) {
      return `[${i}].description ${typeof description === 'string' ? MSG_PLAIN_LINE : MSG_STRING}`
    }
  }
  return undefined
}

/** Why `value` is not a valid answer of `type`; undefined when it is. `options` are a choice's offered values. */
export function checkAnswer(type: FoundationType, value: AnswerValue, options: readonly FoundationOption[] = []): string | undefined {
  if (type === 'bool') return typeof value === 'boolean' ? undefined : MSG_BOOL
  if (type === 'credentialTypes') return checkCredentialTypes(value)
  if (typeof value !== 'string') return MSG_STRING
  if (type === 'url') return checkURL(value)
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
function fill(template: string, values: Record<string, AnswerValue>, foundations: readonly Foundation[]): string | undefined {
  let missing = false
  const filled = template.replace(ANSWER_REFERENCE, (_, id: string) => {
    const value = resolvedAnswer(id, values, foundations)
    missing ||= value === undefined
    return value ?? ''
  })
  return missing ? undefined : filled
}

/** Whether `f`'s default applies: it has one and its `defaultWhen` holds over the resolved answers. */
export function defaultApplies(f: Foundation, values: Record<string, AnswerValue>, foundations: readonly Foundation[]): boolean {
  if (f.default === '') return false
  return !f.defaultWhen || resolvedAnswer(f.defaultWhen.id, values, foundations) === f.defaultWhen.equals
}

/** `f`'s default with its references filled; undefined when it does not apply or a reference is unresolved. */
export function defaultValue(f: Foundation, values: Record<string, AnswerValue>, foundations: readonly Foundation[]): string | undefined {
  return defaultApplies(f, values, foundations) ? fill(f.default, values, foundations) : undefined
}

/** The answer `id` resolves to: the given one, else its applicable default; undefined while neither exists. */
export function resolvedAnswer(id: string, values: Record<string, AnswerValue>, foundations: readonly Foundation[]): string | undefined {
  // The bundle compiler lets only copy targets reference a bool or a list, so neither resolves into a template.
  const given = values[id]
  if (given !== undefined) return typeof given === 'string' ? given : undefined
  const f = foundations.find((o) => o.id === id)
  return f && defaultValue(f, values, foundations)
}

/** Why the given `value` of `f` does not start with its prefix; undefined when it does or the prefix is unresolved. */
function prefixError(f: Foundation, value: AnswerValue, values: Record<string, AnswerValue>, foundations: readonly Foundation[]): string | undefined {
  if (!f.prefix || typeof value !== 'string') return undefined
  const prefix = fill(f.prefix, values, foundations)
  return prefix === undefined || value.startsWith(prefix) ? undefined : msgPrefix(prefix)
}

/** The error of every given answer, by answer id: its format, then its prefix over the well-formed answers. An id the
 *  foundations do not declare is not checked. */
export function answerErrors(values: Record<string, AnswerValue>, foundations: readonly Foundation[] = []): Record<string, string> {
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
export function dropMismatched(
  values: Record<string, AnswerValue>,
  changed: string,
  foundations: readonly Foundation[] = [],
): Record<string, AnswerValue> {
  const stale = foundations.filter((f) => {
    const value = values[f.id]
    if (f.id === changed || value === undefined) return false
    return [...f.prefix.matchAll(ANSWER_REFERENCE)].some(([, id]) => id === changed) && prefixError(f, value, values, foundations) !== undefined
  })
  if (stale.length === 0) return values
  return Object.fromEntries(Object.entries(values).filter(([id]) => !stale.some((f) => f.id === id)))
}

/** The answers the server accepts: an answer with a format error stays on screen but is not sent. */
export function validAnswers(values: Record<string, AnswerValue>, foundations?: readonly Foundation[]): Record<string, AnswerValue> {
  const errors = answerErrors(values, foundations)
  return Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
}

/** A given answer as a text field holds it; undefined when absent or a bool or list, which no text field asks. */
export function textAnswer(value: AnswerValue | undefined): string | undefined {
  return typeof value === 'string' ? value : undefined
}
