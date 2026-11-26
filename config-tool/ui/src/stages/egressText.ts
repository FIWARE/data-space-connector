/** The Egress panel's copy (ADR 0025). */
export const EGRESS_PANEL = 'Egress: proxy and private CA'
export const PREVIEW_HEADING = 'What each component gets'
export const COMPONENT_HEADER = 'Component'
export const PROXY_HEADER = 'Proxy'
export const CA_HEADER = 'Private CA'
export const NOTHING_SET = '—'
export const PROXY_SET = 'set'
export const PROXY_IGNORED = 'ignored: the loaded file sets proxy mappings'
export const CA_TRUSTED = 'trusts ca.crt'
export const CA_KEY_NOTE = 'The Secret needs the key ca.crt with the PEM bundle. APISIX then trusts only that bundle.'
export const SECRETS_HEADING = 'Create before installing'
export const NO_ROUTES =
  'No APISIX route with openid-connect yet, so APISIX gets no proxy and no route verification: they are written into each such route once there is one.'
export const BAE_LOGIN =
  "The marketplace's VC login (token request and key fetch) always goes through the HTTPS proxy, even to an in-cluster verifier: that code ignores No proxy."
export const VC_OPERATOR_CIDR =
  'No proxy holds no CIDR, so the vc-operator reaches the Kubernetes API through the proxy. Append the service CIDR to No proxy.'

/** Matches an IPv4 CIDR entry of a NO_PROXY list. */
export const CIDR = /^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/

/** What the preview says for a mechanism left out because of the element `foreign` (or an edit) in `list`. */
export function caWithheld(reason: string, list: string, foreign?: string): string {
  if (reason === 'collision' && foreign) return `withheld: ${foreign} in ${list} occupies its path`
  if (reason === 'conflict') return 'withheld: the loaded file mounts its own CA bundle'
  return `withheld: the loaded file changed ${list}`
}

/** The pointer to the No proxy findings shown at the field. */
export function noProxyGaps(count: number): string {
  return count === 1 ? 'One in-cluster service is not covered by No proxy; see the field above.' : `${count} in-cluster services are not covered by No proxy; see the field above.`
}

/** One Secret the user creates in the release namespace. */
export function secretLine(namespace: string | undefined, name: string, key: string, purpose: string): string {
  return `Secret ${namespace ? `${namespace}/` : ''}${name}, key ${key}: ${purpose}`
}
