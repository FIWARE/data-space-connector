/** Foundation answer validation and resolution, mirroring `model.CompiledFoundation.Check` and `render.resolve` in Go. */
import type { CredentialType, Foundation, FoundationOption, FoundationType } from '../api/foundations'
import { UNANSWERED, type Answers, type AnswerValue, type Target } from './answers'

/** DNS name limit, `maxHostnameLength` in Go. */
export const MAX_HOSTNAME_LENGTH = 253
/** Longest url answer, `maxURLLength` in Go. */
export const MAX_URL_LENGTH = 2048
/** Longest credentialTypes answer, `maxCredentialTypes` in Go. */
export const MAX_CREDENTIAL_TYPES = 64
/** Longest local part of an email answer, `maxEmailLocalLength` in Go. */
export const MAX_EMAIL_LOCAL_LENGTH = 64

export const MSG_PLAIN_LINE = `must be a single non-empty line without leading or trailing spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_HOSTNAME = `must be a host name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_SECRET_NAME = `must be a Secret name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_RESOURCE_NAME = `must be a Kubernetes name of lowercase letters, digits, '-' and '.', at most ${MAX_HOSTNAME_LENGTH} characters`
export const MSG_DOMAIN = 'must be a domain of at least two labels'
export const MSG_EMAIL_LOCAL = `must be an e-mail address local@domain, the local part at most ${MAX_EMAIL_LOCAL_LENGTH} letters, digits and '.', '_', '%', '+', '-'`
export const MSG_EMAIL_DOMAIN = "must be an e-mail address whose domain has at least two labels of lowercase letters, digits and '-'"
export const MSG_EMAIL_RESERVED = 'must not use an RFC 2606 example or test domain, which no mail reaches'
export const MSG_STRING = 'must be a string'
export const MSG_URL =
  `must be an absolute http or https URL with a host, without user info, fragment, spaces, control characters, '"', '\\', '<' or '>'`
export const MSG_URL_LENGTH = `must be a URL of at most ${MAX_URL_LENGTH} bytes`
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
// Go refuses a malformed % escape before the query, which WHATWG URL keeps as is.
const URL_BAD_ESCAPE = /%(?![0-9A-Fa-f]{2})/
const CREDENTIAL_TYPE = /^[A-Za-z][A-Za-z0-9._-]*$/
const CREDENTIAL_TYPE_KEYS = ['type', 'description']
const EMAIL_LOCAL = /^[A-Za-z0-9_%+-]+(\.[A-Za-z0-9_%+-]+)*$/
// RFC 2606's example second-level domains (their subdomains too) and reserved top-level names, as in Go.
const RESERVED_MAIL_DOMAINS = ['example.com', 'example.net', 'example.org']
const RESERVED_MAIL_TLDS = ['example', 'test', 'invalid', 'localhost']

function checkEmail(value: string): string | undefined {
  const at = value.indexOf('@')
  const local = value.slice(0, at)
  if (at < 0 || local.length > MAX_EMAIL_LOCAL_LENGTH || !EMAIL_LOCAL.test(local)) return MSG_EMAIL_LOCAL
  const domain = value.slice(at + 1)
  if (checkAnswer('domain', domain) !== undefined) return MSG_EMAIL_DOMAIN
  const tld = domain.slice(domain.lastIndexOf('.') + 1)
  if (RESERVED_MAIL_TLDS.includes(tld) || RESERVED_MAIL_DOMAINS.some((d) => domain === d || domain.endsWith(`.${d}`))) return MSG_EMAIL_RESERVED
  return undefined
}

function checkURL(value: string): string | undefined {
  // Go's len counts UTF-8 bytes, not UTF-16 units.
  if (new TextEncoder().encode(value).length > MAX_URL_LENGTH) return MSG_URL_LENGTH
  if (!PLAIN_LINE.test(value) || /\s|#/u.test(value) || !URL_PREFIX.test(value)) return MSG_URL
  // Go refuses any userinfo, even an empty one, which WHATWG URL parsing would hide.
  const rest = value.replace(URL_PREFIX, '')
  const authority = rest.split(/[/?]/, 1)[0] ?? ''
  if (authority.includes('@') || !URL_HOST.test(authority) || URL_HOST_ASCII_ESCAPE.test(authority)) return MSG_URL
  if (URL_BAD_ESCAPE.test(rest.split('?', 1)[0] ?? '')) return MSG_URL
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

const NAME_ERRORS: Partial<Record<FoundationType, string>> = { secretName: MSG_SECRET_NAME, resourceName: MSG_RESOURCE_NAME }

/** Why `value` is not a valid answer of `type`; undefined when it is. `options` are a choice's offered values. */
export function checkAnswer(type: FoundationType, value: AnswerValue, options: readonly FoundationOption[] = []): string | undefined {
  if (type === 'bool') return typeof value === 'boolean' ? undefined : MSG_BOOL
  if (type === 'credentialTypes') return checkCredentialTypes(value)
  if (typeof value !== 'string') return MSG_STRING
  if (type === 'url') return checkURL(value)
  if (type === 'string') return PLAIN_LINE.test(value) ? undefined : MSG_PLAIN_LINE
  if (type === 'choice') return options.some((o) => o.value === value) ? undefined : msgChoice(options)
  if (type === 'did') return DID.test(value) ? undefined : MSG_DID
  if (type === 'email') return checkEmail(value)
  if (value.length > MAX_HOSTNAME_LENGTH || !HOSTNAME.test(value)) return NAME_ERRORS[type] ?? MSG_HOSTNAME
  if (type === 'domain' && !value.includes('.')) return MSG_DOMAIN
  return undefined
}

/** A template's two actions the bundles may use: `{{ fullname .id "chart" }}` (groups 1 and 2) and `{{ .id }}` (group 3). */
const ANSWER_REFERENCE = /\{\{\s*(?:fullname\s+\.([\w.]+)\s+"([^"\\]*)"|\.([\w.]+))\s*\}\}/g

/** The id a match of ANSWER_REFERENCE references. */
function referencedId(match: RegExpMatchArray): string {
  return match[1] ?? match[3] ?? ''
}

/** The read-only ref a default may take from Stage 1's environment, `model.FieldTargetEnvironment` in Go. */
export const TARGET_ENVIRONMENT_REF = 'target.environment'
/** The read-only refs a default may take from Stage 1's release and namespace, `model.RefTargetRelease`/`RefTargetNamespace`. */
export const TARGET_RELEASE_REF = 'target.release'
export const TARGET_NAMESPACE_REF = 'target.namespace'

/** Longest Kubernetes resource name, `maxResourceName` in Go. */
export const MAX_RESOURCE_NAME = 63

/** Values of the reserved refs by id; they are never answers, so they are never saved, counted or checked. */
export type ReservedRefs = Readonly<Record<string, string>>

/** The reserved refs `target` resolves: each of environment, release and namespace once Stage 1 has it. */
export function reservedRefs(target: Pick<Target, 'environment' | 'releaseName' | 'namespace'>): ReservedRefs {
  const refs: Record<string, string> = {}
  if (target.environment !== UNANSWERED) refs[TARGET_ENVIRONMENT_REF] = target.environment
  if (target.releaseName !== '') refs[TARGET_RELEASE_REF] = target.releaseName
  if (target.namespace !== '') refs[TARGET_NAMESPACE_REF] = target.namespace
  return refs
}

/** Helm's standard fullname rule, `model.Fullname` in Go: the release alone when it contains the chart, else
 *  release-chart, truncated to 63 characters without a trailing '-'. */
export function fullname(release: string, chart: string): string {
  const name = (release.includes(chart) ? release : `${release}-${chart}`).slice(0, MAX_RESOURCE_NAME)
  return name.endsWith('-') ? name.slice(0, -1) : name
}

/** `template` with its references filled from the reserved refs, then resolved answers; undefined while one is unresolved. */
export function fill(template: string, values: Record<string, AnswerValue>, foundations: readonly Foundation[], reserved: ReservedRefs): string | undefined {
  let missing = false
  const filled = template.replace(ANSWER_REFERENCE, (_, fullnameOf: string | undefined, chart: string | undefined, id: string | undefined) => {
    const ref = fullnameOf ?? id ?? ''
    const value = reserved[ref] ?? resolvedAnswer(ref, values, foundations, reserved)
    missing ||= value === undefined
    if (value === undefined) return ''
    return chart === undefined ? value : fullname(value, chart)
  })
  return missing ? undefined : filled
}

/** Whether `f`'s default applies: it has one and its `defaultWhen` holds over the resolved answers. */
export function defaultApplies(f: Foundation, values: Record<string, AnswerValue>, foundations: readonly Foundation[], reserved: ReservedRefs): boolean {
  if (f.default === '') return false
  const when = f.defaultWhen
  if (!when) return true
  // A given bool or list resolves too, though resolvedAnswer, which feeds templates, leaves it out.
  if (when.set) return values[when.id] !== undefined || resolvedAnswer(when.id, values, foundations, reserved) !== undefined
  return resolvedAnswer(when.id, values, foundations, reserved) === when.equals
}

/** Whether `f`'s default follows other answers or reserved refs, so dropping a given answer lets it follow them again. */
export function derivedDefault(f: Pick<Foundation, 'default'>): boolean {
  return [...f.default.matchAll(ANSWER_REFERENCE)].length > 0
}

/** `f`'s default with its references filled; undefined when it does not apply or a reference is unresolved. */
export function defaultValue(f: Foundation, values: Record<string, AnswerValue>, foundations: readonly Foundation[], reserved: ReservedRefs): string | undefined {
  return defaultApplies(f, values, foundations, reserved) ? fill(f.default, values, foundations, reserved) : undefined
}

/** The answer `id` resolves to: the given one, else its applicable default; undefined while neither exists. */
export function resolvedAnswer(id: string, values: Record<string, AnswerValue>, foundations: readonly Foundation[], reserved: ReservedRefs): string | undefined {
  // The bundle compiler lets only copy targets reference a bool or a list, so neither resolves into a template.
  const given = values[id]
  if (given !== undefined) return typeof given === 'string' ? given : undefined
  const f = foundations.find((o) => o.id === id)
  return f && defaultValue(f, values, foundations, reserved)
}

/** Why the given `value` of `f` does not start with its prefix; undefined when it does or the prefix is unresolved. */
function prefixError(
  f: Foundation,
  value: AnswerValue,
  values: Record<string, AnswerValue>,
  foundations: readonly Foundation[],
  reserved: ReservedRefs,
): string | undefined {
  if (!f.prefix || typeof value !== 'string') return undefined
  const prefix = fill(f.prefix, values, foundations, reserved)
  return prefix === undefined || value.startsWith(prefix) ? undefined : msgPrefix(prefix)
}

/** The error of every given answer, by answer id: its format, then its prefix over the well-formed answers. An id the
 *  foundations do not declare is not checked. */
export function answerErrors(
  values: Record<string, AnswerValue>,
  foundations: readonly Foundation[] | undefined,
  reserved: ReservedRefs,
): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const f of foundations ?? []) {
    const value = values[f.id]
    const error = value === undefined ? undefined : checkAnswer(f.type, value, f.options)
    if (error !== undefined) errors[f.id] = error
  }
  const wellFormed = Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
  for (const f of foundations ?? []) {
    const value = wellFormed[f.id]
    const error = value === undefined ? undefined : prefixError(f, value, wellFormed, foundations ?? [], reserved)
    if (error !== undefined) errors[f.id] = error
  }
  return errors
}

/** `values` without the answers whose prefix references `changed` and no longer matches, so a derived default can take
 *  over again; the changed answer itself is kept. */
export function dropMismatched(
  values: Record<string, AnswerValue>,
  changed: string,
  foundations: readonly Foundation[] | undefined,
  reserved: ReservedRefs,
): Record<string, AnswerValue> {
  const stale = (foundations ?? []).filter((f) => {
    const value = values[f.id]
    if (f.id === changed || value === undefined) return false
    return [...f.prefix.matchAll(ANSWER_REFERENCE)].some((m) => referencedId(m) === changed) && prefixError(f, value, values, foundations ?? [], reserved) !== undefined
  })
  if (stale.length === 0) return values
  return Object.fromEntries(Object.entries(values).filter(([id]) => !stale.some((f) => f.id === id)))
}

/** The answers the server accepts: an answer with a format error stays on screen but is not sent. */
export function validAnswers(
  values: Record<string, AnswerValue>,
  foundations: readonly Foundation[] | undefined,
  reserved: ReservedRefs,
): Record<string, AnswerValue> {
  const errors = answerErrors(values, foundations, reserved)
  return Object.fromEntries(Object.entries(values).filter(([id]) => errors[id] === undefined))
}

/** A given answer as a text field holds it; undefined when absent or a bool or list, which no text field asks. */
export function textAnswer(value: AnswerValue | undefined): string | undefined {
  return typeof value === 'string' ? value : undefined
}

/** Whether a feature `f` is gated on is off, which hides it on Stage 3 (display only, the render ignores it).
 *  Nothing is off while no role is picked; after one, an absent feature is not offered and so off. */
export function featureOff(f: Pick<Foundation, 'features'>, answers: Pick<Answers, 'role' | 'features'>): boolean {
  return answers.role !== undefined && f.features.some((id) => answers.features[id]?.enabled !== true)
}
