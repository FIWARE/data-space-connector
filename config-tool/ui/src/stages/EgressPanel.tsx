/** S3's Egress panel: the forward proxy and the private CA Secret, and what each component gets from them (ADR 0025). */
import type { ReactNode } from 'react'
import type { Foundation } from '../api/foundations'
import type { Finding, Leaf, RequiredSecret, Withheld } from '../api/render'
import { useStore } from '../state/context'
import { reservedRefs, resolvedAnswer, validAnswers } from '../state/foundations'
import {
  BAE_LOGIN,
  CA_HEADER,
  CA_KEY_NOTE,
  CA_NOT_IN_EFFECT,
  CA_TRUSTED,
  CIDR,
  COMPONENT_HEADER,
  EGRESS_PANEL,
  NO_ROUTES,
  noProxyGaps,
  NOTHING_SET,
  PREVIEW_HEADING,
  PROXY_HEADER,
  PROXY_IGNORED,
  PROXY_SET,
  SECRETS_HEADING,
  secretLine,
  VC_OPERATOR_CIDR,
  withheldCell,
} from './egressText'

const PROXY_HTTPS = 'proxy.https'
const PROXY_NO = 'proxy.no'
const CA_SECRET = 'ca.privateSecret'
const PROXY_GROUP_PREFIX = 'proxy.'
const RULE_PROXY_MAPPINGS = 'keycloak-proxy-mappings'
const RULE_NO_PROXY_COVERAGE = 'no-proxy-coverage'
const APISIX_ROUTES = 'decentralizedIam.odrlAuthorization.apisix.routes['
const OIDC_PLUGIN = '.plugins.openid-connect.'

/** A component the egress foundations reach, by the values prefix its targets lie under. */
interface EgressComponent {
  id: string
  label: string
  prefix: string
}

const VERIFIER: EgressComponent = { id: 'verifier', label: 'Verifier', prefix: 'decentralizedIam.vcAuthentication.vcverifier.' }
const APISIX: EgressComponent = { id: 'apisix', label: 'APISIX', prefix: 'decentralizedIam.odrlAuthorization.apisix.' }
const KEYCLOAK: EgressComponent = { id: 'keycloak', label: 'Keycloak', prefix: 'keycloak.' }
const MARKETPLACE: EgressComponent = { id: 'marketplace', label: 'Marketplace', prefix: 'marketplace.' }
const VC_OPERATOR: EgressComponent = { id: 'vcOperator', label: 'vc-operator', prefix: 'vc-operator.' }
const COMPONENTS = [VERIFIER, APISIX, KEYCLOAK, MARKETPLACE, VC_OPERATOR]

/** What the last render left for the preview: its leaves, the withheld mechanisms and the required Secrets. */
export interface EgressRender {
  leaves: readonly Leaf[]
  withheld: readonly Withheld[]
  secrets: readonly RequiredSecret[]
}

/** The leaves the render owns, not pinned, of the foundations `owns` accepts. */
function owned(leaves: readonly Leaf[], owns: (foundation: string) => boolean): Leaf[] {
  return leaves.filter((l) => !l.provenance.origin && !l.provenance.pinned && owns(l.provenance.foundation))
}

const under = (leaves: readonly Leaf[], c: EgressComponent) => leaves.some((l) => l.path.startsWith(c.prefix))

/** A pinned leaf whose kept value is not what the render would write, mirroring `inEffect` in `internal/check`. */
const overridden = (l: Leaf) => l.provenance.pinned && !(l.provenance.kept && l.value === l.provenance.rendered)

/** The withheld entry under `c`; every bundle mechanism is a CA one, so `ca` picks those and !`ca` the proxy elements. */
function withheldUnder(withheld: readonly Withheld[], c: EgressComponent, ca: boolean): Withheld | undefined {
  return withheld.find((w) => (w.mechanism !== '') === ca && (w.list || w.conflict?.[0] || '').startsWith(c.prefix))
}

/** The per-component table and the notes on what the proxy and the CA do not reach. */
function Preview({ rendered, findings }: { rendered: EgressRender; findings: readonly Finding[] }) {
  const { state } = useStore()
  const foundations = state.foundations ?? []
  const reserved = reservedRefs(state.answers.target)
  const values = validAnswers(state.answers.values, foundations, reserved)
  const noProxy = resolvedAnswer(PROXY_NO, values, foundations, reserved)
  const proxyOn = resolvedAnswer(PROXY_HTTPS, values, foundations, reserved) !== undefined && noProxy !== undefined
  const caOn = values[CA_SECRET] !== undefined
  if (!proxyOn && !caOn) return null

  const proxy = owned(rendered.leaves, (f) => f.startsWith(PROXY_GROUP_PREFIX))
  const ca = owned(rendered.leaves, (f) => f === CA_SECRET)
  // Any provenance: a route whose loaded ssl_verify or proxy_opts is pinned is still a route.
  const routes = rendered.leaves.some((l) => l.path.startsWith(APISIX_ROUTES) && l.path.includes(OIDC_PLUGIN))
  const caOverridden = rendered.leaves.filter((l) => l.provenance.foundation === CA_SECRET && overridden(l))
  const mappings = findings.some((f) => f.rule === RULE_PROXY_MAPPINGS)
  const gaps = findings.filter((f) => f.rule === RULE_NO_PROXY_COVERAGE).length
  const cidr = (noProxy ?? '').split(',').some((e) => CIDR.test(e.trim()))

  const proxyCell = (c: EgressComponent) => {
    const w = withheldUnder(rendered.withheld, c, false)
    if (w) return withheldCell(w.reason, w.list, w.foreignKey)
    if (!under(proxy, c)) return NOTHING_SET
    return c === KEYCLOAK && mappings ? PROXY_IGNORED : PROXY_SET
  }
  const caCell = (c: EgressComponent) => {
    const w = withheldUnder(rendered.withheld, c, true)
    if (w) return withheldCell(w.reason, w.list, w.foreignKey)
    if (under(caOverridden, c)) return CA_NOT_IN_EFFECT
    return under(ca, c) ? CA_TRUSTED : NOTHING_SET
  }
  const notes = [
    !routes && NO_ROUTES,
    proxyOn && under(proxy, MARKETPLACE) && BAE_LOGIN,
    proxyOn && under(proxy, VC_OPERATOR) && !cidr && VC_OPERATOR_CIDR,
    gaps > 0 && noProxyGaps(gaps),
  ].filter((n): n is string => typeof n === 'string')

  return (
    <div role="group" aria-labelledby="egress-preview">
      <h4 id="egress-preview">{PREVIEW_HEADING}</h4>
      <table>
        <thead>
          <tr>
            <th scope="col">{COMPONENT_HEADER}</th>
            <th scope="col">{PROXY_HEADER}</th>
            <th scope="col">{CA_HEADER}</th>
          </tr>
        </thead>
        <tbody>
          {COMPONENTS.map((c) => (
            <tr key={c.id}>
              <th scope="row">{c.label}</th>
              <td>{proxyOn ? proxyCell(c) : NOTHING_SET}</td>
              <td>{caOn ? caCell(c) : NOTHING_SET}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {notes.map((n) => (
        <p key={n} className="hint">
          {n}
        </p>
      ))}
    </div>
  )
}

/** The Secrets the values reference, which the user creates in the release namespace before installing. */
function Secrets({ secrets }: { secrets: readonly RequiredSecret[] }) {
  if (secrets.length === 0) return null
  return (
    <div role="group" aria-labelledby="egress-secrets">
      <h4 id="egress-secrets">{SECRETS_HEADING}</h4>
      <ul>
        {secrets.map((s) => (
          <li key={`${s.name}#${s.key}`}>{secretLine(s.namespace, s.name, s.key, s.purpose)}</li>
        ))}
      </ul>
    </div>
  )
}

/** The Egress panel; `field` renders one foundation's field, so derived answers get the same field as elsewhere. */
export function EgressPanel({
  foundations,
  findings,
  rendered,
  field,
}: {
  foundations: readonly Foundation[]
  findings: readonly Finding[]
  rendered: EgressRender
  field: (f: Foundation) => ReactNode
}) {
  return (
    <div className="panel" role="group" aria-labelledby="foundations-egress">
      <h3 id="foundations-egress">{EGRESS_PANEL}</h3>
      {foundations.map((f) => (
        <div key={f.id}>
          {field(f)}
          {f.id === CA_SECRET && <p className="hint">{CA_KEY_NOTE}</p>}
        </div>
      ))}
      <Preview rendered={rendered} findings={findings} />
      <Secrets secrets={rendered.secrets} />
    </div>
  )
}
