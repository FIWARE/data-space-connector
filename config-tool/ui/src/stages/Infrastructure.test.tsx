import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Dependency, Mode } from '../api/infrastructure'
import type { Finding, RenderResult } from '../api/render'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import type { AnswerValue } from '../state/answers'
import { MSG_EMAIL_RESERVED } from '../state/foundations'
import { fieldElementId, type QuestionId } from '../state/openQuestions'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { CERT_MANAGER, FOUNDATIONS, mockApi } from '../test/api'
import { DERIVED_BADGE, DEVELOPMENT_BADGE, PRODUCTION_LINK, WARNING_LINK, writesListLabel, writesText } from './infrastructureText'

const NAMESPACE = 'dsc'
const DOMAIN = 'provider.org'
const TITLE = CERT_MANAGER.title
const BUNDLED = 'Install cert-manager with the connector'
const CLUSTER = 'Use the cert-manager already in the cluster'
const EXTERNAL = 'I provide a TLS Secret per host'
const ACME_FIELD = 'infrastructure.certManager.acmeEmail'
const PRODUCTION_DOC = 'doc/deployment-integration/roles/PREREQUISITES.md#production'

function onInfrastructure(values: Record<string, AnswerValue>): AppState {
  return {
    ...initialState,
    entered: true,
    stage: StageId.Infrastructure,
    visited: [StageId.Target, StageId.Role, StageId.Foundations, StageId.Infrastructure],
    answers: {
      ...initialState.answers,
      role: 'provider',
      target: { ...initialState.answers.target, releaseName: 'provider', namespace: NAMESPACE },
      values,
    },
  }
}

/** A render that only reports `findings`; Stage 4 shows nothing of the tree. */
function findingsRender(findings: Finding[]) {
  return (): RenderResult => ({ tree: { leaves: [] }, diff: { groups: [] }, derived: { rederived: [], pinned: [] }, handSet: [], findings, secrets: [] })
}

function renderStage({ dependencies = [CERT_MANAGER], findings = [] }: { dependencies?: Dependency[]; findings?: Finding[] } = {}) {
  mockApi(undefined, undefined, undefined, {}, {}, {}, { foundations: () => FOUNDATIONS, render: findingsRender(findings) }, {}, () => ({ dependencies }))
  render(
    <StoreProvider initial={onInfrastructure({ 'domain.base': DOMAIN })}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

function modeOf(id: string): Mode {
  const mode = CERT_MANAGER.modes.find((m) => m.id === id)
  if (!mode) throw new Error(`no mode ${id}`)
  return mode
}

const card = () => screen.findByRole('region', { name: TITLE })
const choose = async (mode: string) => userEvent.click(within(await card()).getByRole('radio', { name: mode }))
const textboxes = async () => within(await card()).queryAllByRole('textbox').map((t) => t.id)
const openCount = () => Number(/○ (\d+) open questions/.exec(screen.getByText(/open questions/).textContent ?? '')?.[1])

describe('Infrastructure', () => {
  it('offers the three modes as a radio group with none chosen and an Ask later on the mode', async () => {
    renderStage()
    const group = await card()
    const radios = within(group).getAllByRole('radio', { name: /cert-manager|TLS Secret/ })
    expect(radios.map((r) => r.getAttribute('value'))).toEqual(['bundled', 'cluster', 'external'])
    expect(radios.every((r) => !(r as HTMLInputElement).checked)).toBe(true)
    expect(within(group).getByRole('button', { name: `Ask ${TITLE} later` })).toHaveAttribute('aria-pressed', 'false')
    expect(within(group).queryByRole('textbox')).toBeNull()
  })

  it.each([
    [BUNDLED, []],
    [CLUSTER, ['infrastructure.certManager.issuer']],
    [EXTERNAL, ['infrastructure.certManager.secret.apisix', 'infrastructure.certManager.secret.keycloak']],
  ])('choosing "%s" reveals exactly its text fields', async (mode, fields) => {
    renderStage()
    await choose(mode)
    expect(await textboxes()).toEqual(fields.map((f) => fieldElementId(f as QuestionId)))
  })

  it('bundled warns with the install-once link, derives the namespace and asks ACME only for Let’s Encrypt', async () => {
    renderStage()
    await choose(BUNDLED)
    const group = await card()
    expect(within(group).getByRole('note')).toHaveTextContent('A cluster can only have one cert-manager.')
    expect(within(group).getByRole('link', { name: WARNING_LINK })).toHaveAttribute('href', expect.stringContaining('PREREQUISITES.md#installing-the-operators-once'))
    expect(within(group).getByRole('radio', { name: 'Self-signed CA' })).toBeChecked()
    expect(within(group).getByLabelText('Namespace')).toHaveTextContent(NAMESPACE)
    expect(within(group).getAllByText(DERIVED_BADGE)).not.toHaveLength(0)
    expect(within(group).queryByLabelText('ACME contact')).toBeNull()
    expect(within(group).queryByLabelText('Ingress class')).toBeNull()

    await userEvent.click(within(group).getByRole('radio', { name: "Let's Encrypt" }))
    expect(within(group).getByLabelText('ACME contact')).toHaveValue('')
    expect(within(group).getByLabelText('Ingress class')).toHaveValue('')
    expect(within(group).queryByLabelText('Namespace')).toBeNull()
  })

  it('external lists one Secret per ingress, defaulted from its host', async () => {
    renderStage()
    await choose(EXTERNAL)
    const group = await card()
    expect(within(group).getByLabelText('APISIX')).toHaveValue('apisix-provider-tls')
    expect(within(group).getByLabelText('Keycloak')).toHaveValue(`keycloak.${DOMAIN}-tls`)
  })

  it('lists the chosen mode’s keys in the writes disclosure', async () => {
    renderStage()
    await choose(CLUSTER)
    const group = await card()
    const cluster = modeOf('cluster')
    expect(within(group).getByText(writesText(cluster.keys.length))).toBeInTheDocument()
    const list = within(group).getByRole('list', { name: writesListLabel(CLUSTER), hidden: true })
    expect(within(list).getAllByRole('listitem', { hidden: true }).map((li) => li.textContent)).toEqual(cluster.keys.map((k) => k.path))
  })

  it('hides the previous mode’s fields on a switch but keeps their values', async () => {
    renderStage()
    await choose(CLUSTER)
    await userEvent.type(within(await card()).getByLabelText('ClusterIssuer'), 'letsencrypt')
    await choose(EXTERNAL)
    expect(within(await card()).queryByLabelText('ClusterIssuer')).toBeNull()
    await choose(CLUSTER)
    expect(within(await card()).getByLabelText('ClusterIssuer')).toHaveValue('letsencrypt')
  })

  it('counts the unchosen mode and the active mode’s open questions, and Ask later marks an answered one', async () => {
    renderStage()
    await card()
    const unchosen = openCount()
    await choose(CLUSTER)
    // The mode is answered, its one question is not.
    expect(openCount()).toBe(unchosen)
    const group = await card()
    await userEvent.type(within(group).getByLabelText('ClusterIssuer'), 'letsencrypt')
    expect(openCount()).toBe(unchosen - 1)
    const ask = within(group).getByRole('button', { name: `Ask ${TITLE}: ClusterIssuer later` })
    await userEvent.click(ask)
    expect(ask).toHaveAttribute('aria-pressed', 'true')
    expect(openCount()).toBe(unchosen)
    // Another mode's questions never count.
    await choose(EXTERNAL)
    expect(openCount()).toBe(unchosen - 1)
  })

  it('shows a refused value and a finding on the field its Ask later marks', async () => {
    const message = 'Let’s Encrypt needs a contact address.'
    const finding: Finding = { rule: 'acme-email-for-prod-issuer', severity: 'error', stage: 'infrastructure', field: ACME_FIELD, message, doc: '' }
    renderStage({ findings: [finding] })
    await choose(BUNDLED)
    await userEvent.click(within(await card()).getByRole('radio', { name: "Let's Encrypt" }))
    const input = within(await card()).getByLabelText('ACME contact')
    await userEvent.type(input, 'admins@example.com')
    expect(input).toHaveAttribute('id', fieldElementId(ACME_FIELD))
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(input).toHaveAccessibleDescription(expect.stringContaining(message)))
    expect(input).toHaveAccessibleDescription(expect.stringContaining(MSG_EMAIL_RESERVED))
    const row = input.closest('.row') as HTMLElement
    expect(within(row).getByRole('button', { name: `Ask ${TITLE}: ACME contact later` })).toBeInTheDocument()
  })

  it.each([
    [false, 0],
    [true, 1],
  ])('marks a development mode (%s) with the badge and the production link', async (development, shown) => {
    const modes = CERT_MANAGER.modes.map((m) => (m.id === 'bundled' ? { ...m, development, productionDoc: development ? PRODUCTION_DOC : '' } : m))
    const dependency: Dependency = { ...CERT_MANAGER, modes }
    renderStage({ dependencies: [dependency] })
    const group = await card()
    expect(within(group).queryAllByText(DEVELOPMENT_BADGE)).toHaveLength(shown)
    const links = within(group).queryAllByRole('link', { name: PRODUCTION_LINK })
    expect(links).toHaveLength(shown)
    links.forEach((l) => expect(l).toHaveAttribute('href', expect.stringContaining(PRODUCTION_DOC)))
  })
})
