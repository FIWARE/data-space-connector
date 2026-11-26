import { render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Finding, Leaf, RenderResult, RequiredSecret, Withheld } from '../api/render'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import type { AnswerValue } from '../state/answers'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { EGRESS_FOUNDATIONS, mockApi, type RenderBody } from '../test/api'
import {
  BAE_LOGIN,
  CA_KEY_NOTE,
  CA_TRUSTED,
  caWithheld,
  EGRESS_PANEL,
  NO_ROUTES,
  noProxyGaps,
  NOTHING_SET,
  PREVIEW_HEADING,
  PROXY_IGNORED,
  PROXY_SET,
  SECRETS_HEADING,
  secretLine,
  VC_OPERATOR_CIDR,
} from './egressText'

const RELEASE = 'provider'
const NAMESPACE = 'dsc'
const PROXY = 'http://proxy.corp.example:3128'
const CA = 'corp-ca'
const VERIFIER_ENV = 'decentralizedIam.vcAuthentication.vcverifier.deployment.additionalEnvVars'
const VERIFIER_MOUNTS = 'decentralizedIam.vcAuthentication.vcverifier.deployment.additionalVolumeMounts'
const ROUTE_PROXY = 'decentralizedIam.odrlAuthorization.apisix.routes[0].plugins.openid-connect.proxy_opts.https_proxy'
const CA_SECRET: RequiredSecret = { namespace: NAMESPACE, name: CA, key: 'ca.crt', purpose: 'The PEM bundle of the private CA.' }

const leaf = (path: string, value: unknown, foundation: string, element?: string): Leaf => ({
  path,
  value,
  provenance: { foundation, answers: [foundation], fn: 'copy', target: 0, pinned: false, kept: false, element },
})

/** What the fake render adds to the egress leaves the answers give. */
interface Extra {
  routes?: boolean
  withheld?: Withheld[]
  findings?: Finding[]
}

/** The egress leaves a render of `answers` writes into each component, the APISIX routes only with `routes`. */
function egressRender({ routes = false, withheld = [], findings = [] }: Extra) {
  return (body: RenderBody): RenderResult => {
    const a = body.profile.answers ?? {}
    const leaves: Leaf[] = []
    if (typeof a['proxy.https'] === 'string') {
      leaves.push(
        leaf(`${VERIFIER_ENV}[0].value`, a['proxy.https'], 'proxy.https', 'HTTPS_PROXY'),
        leaf('keycloak.proxy.httpsProxy', a['proxy.https'], 'proxy.https'),
        leaf('marketplace.bizEcosystemLogicProxy.additionalEnvVars[2].value', a['proxy.https'], 'proxy.https', 'HTTPS_PROXY'),
        leaf('vc-operator.extraEnvVars[0].value', a['proxy.https'], 'proxy.https', 'HTTPS_PROXY'),
      )
      if (routes) leaves.push(leaf(ROUTE_PROXY, a['proxy.https'], 'proxy.https'))
    }
    if (typeof a['ca.privateSecret'] === 'string') {
      leaves.push(
        leaf('decentralizedIam.odrlAuthorization.apisix.apisix.ssl.existingCASecret', a['ca.privateSecret'], 'ca.privateSecret'),
        leaf('keycloak.extraVolumes[0].secret.secretName', a['ca.privateSecret'], 'ca.privateSecret', 'private-ca'),
      )
    }
    return {
      tree: { leaves: leaves.sort((x, y) => x.path.localeCompare(y.path)), withheld },
      diff: { groups: [] },
      derived: { rederived: [], pinned: [] },
      handSet: [],
      findings,
      secrets: typeof a['ca.privateSecret'] === 'string' ? [CA_SECRET] : [],
    }
  }
}

function onFoundations(values: Record<string, AnswerValue>): AppState {
  return {
    ...initialState,
    entered: true,
    stage: StageId.Foundations,
    visited: [StageId.Target, StageId.Role, StageId.Foundations],
    answers: {
      ...initialState.answers,
      role: 'provider',
      target: { ...initialState.answers.target, releaseName: RELEASE, namespace: NAMESPACE },
      values,
    },
  }
}

function renderPanel(values: Record<string, AnswerValue>, extra: Extra = {}) {
  mockApi(undefined, undefined, undefined, {}, {}, {}, { foundations: () => EGRESS_FOUNDATIONS, render: egressRender(extra) })
  render(
    <StoreProvider initial={onFoundations(values)}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

const panel = () => screen.findByRole('group', { name: EGRESS_PANEL })
const preview = async () => within(await panel()).findByRole('group', { name: PREVIEW_HEADING })

/** The preview's rows as [component, proxy, CA]. */
function rowsOf(group: HTMLElement): string[][] {
  return within(within(group).getByRole('table'))
    .getAllByRole('row')
    .slice(1)
    .map((row) => [...row.querySelectorAll('th, td')].map((c) => c.textContent ?? ''))
}

const collision: Withheld = {
  reason: 'collision',
  mechanism: 'verifier-ca',
  element: 'private-ca-bundle',
  list: VERIFIER_MOUNTS,
  field: 'mountPath',
  value: '/etc/ssl/cert.pem',
  foreignKey: 'root-ca',
}

const finding = (rule: string, field: string): Finding => ({
  rule,
  severity: 'warning',
  stage: 'foundations',
  field,
  message: `${rule} message`,
  doc: 'doc/deployment-integration/roles/PREREQUISITES.md#outbound-access',
})

describe('EgressPanel', () => {
  it('asks the proxy answers directly and previews nothing while no proxy or CA is given', async () => {
    renderPanel({})
    const group = await panel()
    for (const label of ['HTTPS proxy', 'HTTP proxy', 'No proxy', 'Private CA Secret']) {
      expect(within(group).getByLabelText(label)).toHaveValue('')
    }
    expect(within(group).getByText(CA_KEY_NOTE)).toBeInTheDocument()
    expect(within(group).queryByRole('group', { name: PREVIEW_HEADING })).toBeNull()
    expect(within(group).queryByRole('group', { name: SECRETS_HEADING })).toBeNull()
  })

  it('derives HTTP proxy and No proxy from the HTTPS proxy and the release', async () => {
    renderPanel({ 'proxy.https': PROXY })
    const group = await panel()
    expect(within(group).getByLabelText('HTTP proxy')).toHaveTextContent(PROXY)
    expect(within(group).getByLabelText('No proxy')).toHaveTextContent(`localhost,${NAMESPACE}.svc,${RELEASE}-keycloak`)
  })

  it.each([
    {
      name: 'proxy without CIDR or routes',
      values: { 'proxy.https': PROXY },
      extra: {},
      rows: [
        ['Verifier', PROXY_SET, NOTHING_SET],
        ['APISIX', NOTHING_SET, NOTHING_SET],
        ['Keycloak', PROXY_SET, NOTHING_SET],
        ['Marketplace', PROXY_SET, NOTHING_SET],
        ['vc-operator', PROXY_SET, NOTHING_SET],
      ],
      notes: [NO_ROUTES, BAE_LOGIN, VC_OPERATOR_CIDR],
    },
    {
      name: 'proxy with CIDR and a route',
      values: { 'proxy.https': PROXY, 'proxy.no': 'localhost,10.42.0.0/16,10.43.0.0/16' },
      extra: { routes: true },
      rows: [
        ['Verifier', PROXY_SET, NOTHING_SET],
        ['APISIX', PROXY_SET, NOTHING_SET],
        ['Keycloak', PROXY_SET, NOTHING_SET],
        ['Marketplace', PROXY_SET, NOTHING_SET],
        ['vc-operator', PROXY_SET, NOTHING_SET],
      ],
      notes: [BAE_LOGIN],
    },
    {
      name: 'proxy ignored by Keycloak and uncovered services',
      values: { 'proxy.https': PROXY, 'proxy.no': 'localhost,10.43.0.0/16' },
      extra: {
        routes: true,
        findings: [
          finding('keycloak-proxy-mappings', 'answers.proxy.https'),
          finding('no-proxy-coverage', 'answers.proxy.no'),
          finding('no-proxy-coverage', 'answers.proxy.no'),
        ],
      },
      rows: [
        ['Verifier', PROXY_SET, NOTHING_SET],
        ['APISIX', PROXY_SET, NOTHING_SET],
        ['Keycloak', PROXY_IGNORED, NOTHING_SET],
        ['Marketplace', PROXY_SET, NOTHING_SET],
        ['vc-operator', PROXY_SET, NOTHING_SET],
      ],
      notes: [BAE_LOGIN, noProxyGaps(2)],
    },
    {
      name: 'private CA with a withheld verifier mechanism',
      values: { 'ca.privateSecret': CA },
      extra: { withheld: [collision] },
      rows: [
        ['Verifier', NOTHING_SET, caWithheld('collision', VERIFIER_MOUNTS, 'root-ca')],
        ['APISIX', NOTHING_SET, CA_TRUSTED],
        ['Keycloak', NOTHING_SET, CA_TRUSTED],
        ['Marketplace', NOTHING_SET, NOTHING_SET],
        ['vc-operator', NOTHING_SET, NOTHING_SET],
      ],
      notes: [NO_ROUTES],
    },
  ] as const)('previews the fan-out: $name', async ({ values, extra, rows, notes }) => {
    renderPanel(values, extra as Extra)
    const group = await preview()
    // The table shows before the first render answers, so wait for its leaves.
    await waitFor(() => expect(rowsOf(group)).toEqual(rows))
    for (const n of [NO_ROUTES, BAE_LOGIN, VC_OPERATOR_CIDR, noProxyGaps(2)]) {
      expect(within(group).queryByText(n) !== null).toBe((notes as readonly string[]).includes(n))
    }
  })

  it('lists the CA Secret the user creates before installing', async () => {
    renderPanel({ 'ca.privateSecret': CA })
    const secrets = await within(await panel()).findByRole('group', { name: SECRETS_HEADING })
    expect(within(secrets).getByText(secretLine(NAMESPACE, CA, CA_SECRET.key, CA_SECRET.purpose))).toBeInTheDocument()
  })
})
