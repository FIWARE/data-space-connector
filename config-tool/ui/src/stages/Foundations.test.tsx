import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Foundation } from '../api/foundations'
import type { RenderResult } from '../api/render'
import { Chrome } from '../chrome/Chrome'
import { PUBLIC_NAMES_URL, repositoryDocUrl } from '../links'
import { STAGES } from '../stages'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { MSG_DOMAIN, MSG_HOSTNAME, MSG_PLAIN_LINE } from '../state/foundations'
import { StoreProvider } from '../state/store'
import { DID_FOUNDATIONS, DID_METHOD, DID_VALUE, DOMAIN_WARNING, FOUNDATIONS, mockApi, renderWithFindings, type FoundationFakes, type RenderBody } from '../test/api'
import { READ_MORE } from './findingsText'
import { usedByText } from './foundationText'
import {
  DERIVED_PENDING,
  DIFF_HEADING,
  EDIT_LABEL,
  FOUNDATIONS_HEADING,
  HOSTS_PENDING,
  PINNED_LABEL,
  RENDER_DEBOUNCE_MS,
  RESET_LABEL,
  RESOLVE_LINK,
} from './Foundations'

function foundation(id: string): Foundation {
  const found = FOUNDATIONS.foundations.find((f) => f.id === id)
  if (!found) throw new Error(`no foundation ${id}`)
  return found
}

const DISPLAY_NAME = foundation('org.displayName')
const BASE_DOMAIN = foundation('domain.base')
const KEYCLOAK_HOST = foundation('host.keycloak')
const BASE = 'example.org'
const OLD_BASE = 'old.org'
const FRONTEND_URL = 'keycloak.realm.frontendUrl'

const ON_FOUNDATIONS: AppState = {
  ...initialState,
  entered: true,
  stage: StageId.Foundations,
  visited: [StageId.Target, StageId.Role, StageId.Foundations],
  answers: { ...initialState.answers, role: 'provider' },
}

function renderFoundations(state: AppState = ON_FOUNDATIONS, foundations: FoundationFakes = {}) {
  const fetchMock = mockApi(undefined, undefined, undefined, {}, {}, {}, foundations)
  render(
    <StoreProvider initial={state}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
  return fetchMock
}

const pastDebounce = () => new Promise((resolve) => setTimeout(resolve, RENDER_DEBOUNCE_MS * 2))
const panel = (name: string) => screen.getByRole('group', { name })
const domainInput = () => screen.findByLabelText(BASE_DOMAIN.label)

describe('Foundations', () => {
  it('shows each answer with the count of values and components it reaches', async () => {
    renderFoundations()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(FOUNDATIONS_HEADING)
    await screen.findByLabelText(DISPLAY_NAME.label)
    expect(within(panel('Organization')).getByRole('button', { name: 'used by 1 value in 1 component' })).toBeInTheDocument()
    expect(within(panel('Network')).getByRole('button', { name: 'used by 3 values in 2 components' })).toBeInTheDocument()
  })

  it('lists the target paths only on demand', async () => {
    renderFoundations()
    await domainInput()
    const used = within(panel('Network')).getByRole('button', { name: usedByText(BASE_DOMAIN) })
    expect(used).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('list', { name: `Where ${BASE_DOMAIN.label} is used` })).not.toBeInTheDocument()
    await userEvent.click(used)
    const list = screen.getByRole('list', { name: `Where ${BASE_DOMAIN.label} is used` })
    expect(within(list).getAllByRole('listitem').map((li) => li.textContent)).toEqual(BASE_DOMAIN.targets.map((t) => t.path))
  })

  it('derives the hosts from the base domain once it is answered', async () => {
    renderFoundations()
    const hosts = within(await screen.findByRole('group', { name: /Host names/ }))
    expect(hosts.getByRole('row', { name: new RegExp(KEYCLOAK_HOST.label) })).toHaveTextContent(HOSTS_PENDING)
    await userEvent.type(await domainInput(), BASE)
    expect(hosts.getByRole('row', { name: new RegExp(KEYCLOAK_HOST.label) })).toHaveTextContent(`keycloak.${BASE}`)
  })

  it('shows the hosts as pending while the base domain is invalid, as the render writes none', async () => {
    renderFoundations()
    const hosts = within(await screen.findByRole('group', { name: /Host names/ }))
    await userEvent.type(await domainInput(), 'Example.org')
    expect(hosts.getByRole('row', { name: new RegExp(KEYCLOAK_HOST.label) })).toHaveTextContent(HOSTS_PENDING)
  })

  it.each([
    [DISPLAY_NAME.label, repositoryDocUrl(DISPLAY_NAME.doc)],
    [BASE_DOMAIN.label, repositoryDocUrl(BASE_DOMAIN.doc)],
    [KEYCLOAK_HOST.label, repositoryDocUrl(KEYCLOAK_HOST.doc)],
  ])('links the documentation of %s', async (label, href) => {
    renderFoundations()
    expect(await screen.findByRole('link', { name: `About ${label} ↗` })).toHaveAttribute('href', href)
  })

  it('links why public names have to resolve inside the cluster', async () => {
    renderFoundations()
    expect(await screen.findByRole('link', { name: RESOLVE_LINK })).toHaveAttribute('href', PUBLIC_NAMES_URL)
  })

  it('carries a marked answer into Review and back to its field', async () => {
    renderFoundations()
    await domainInput()
    await userEvent.click(screen.getByRole('button', { name: `Ask ${BASE_DOMAIN.label} later` }))
    await userEvent.click(within(screen.getByRole('navigation', { name: 'Stages' })).getByRole('button', { name: /Review/ }))
    const open = screen.getByRole('heading', { name: /Open questions/ }).closest('div') as HTMLElement
    expect(within(open).getByText(BASE_DOMAIN.label)).toBeInTheDocument()
    await userEvent.click(within(open).getByRole('button', { name: `Go to ${BASE_DOMAIN.label}` }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(FOUNDATIONS_HEADING)
    await waitFor(() => expect(screen.getByLabelText(BASE_DOMAIN.label)).toHaveFocus())
  })

  it('renders on entry as the baseline without a diff and shows the grouped diff after a domain change', async () => {
    const answered = { ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values: { 'domain.base': OLD_BASE } } }
    const fetchMock = renderFoundations(answered)
    const renders = () => fetchMock.mock.calls.filter(([url]) => url === '/api/render')
    await waitFor(() => expect(renders()).toHaveLength(1))
    // The entry render adds every leaf against the server's empty tree; that is not an edit, so it is not shown.
    await screen.findByLabelText(BASE_DOMAIN.label)
    expect(screen.queryByRole('group', { name: DIFF_HEADING })).not.toBeInTheDocument()
    await userEvent.clear(await domainInput())
    await userEvent.type(await domainInput(), BASE)
    const diff = within(await screen.findByRole('group', { name: DIFF_HEADING }))
    const keycloak = within(diff.getByRole('group', { name: KEYCLOAK_HOST.label }))
    expect(keycloak.getByText(FRONTEND_URL).closest('li')).toHaveTextContent(`https://keycloak.${OLD_BASE} → https://keycloak.${BASE}`)
    const last = JSON.parse(String((renders().at(-1)?.[1] as RequestInit).body)) as RenderBody
    expect(last.profile.answers).toEqual({ 'domain.base': BASE })
  })

  it.each([
    [BASE_DOMAIN.label, 'Example.org', MSG_HOSTNAME],
    [BASE_DOMAIN.label, 'localhost', MSG_DOMAIN],
    [DISPLAY_NAME.label, 'Acme ', MSG_PLAIN_LINE],
  ])('shows a format error on %s %j, counts it and does not render it', async (label, value, message) => {
    const fetchMock = renderFoundations()
    const renders = () => fetchMock.mock.calls.filter(([url]) => url === '/api/render')
    const input = await screen.findByLabelText(label)
    await waitFor(() => expect(renders()).toHaveLength(1))
    await userEvent.type(input, value)
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAccessibleDescription(expect.stringContaining(message))
    expect(screen.getByText(/✖ \d+ errors/)).toHaveTextContent('✖ 1 errors')
    await pastDebounce()
    expect(renders()).toHaveLength(1)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps the last valid render as the baseline while an intermediate value is invalid', async () => {
    const answered = { ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values: { 'domain.base': OLD_BASE } } }
    const fetchMock = renderFoundations(answered)
    const renders = () => fetchMock.mock.calls.filter(([url]) => url === '/api/render')
    await waitFor(() => expect(renders()).toHaveLength(1))
    await userEvent.clear(await domainInput())
    await userEvent.type(await domainInput(), 'example')
    await pastDebounce()
    expect(renders()).toHaveLength(1)
    expect(screen.queryByRole('group', { name: DIFF_HEADING })).not.toBeInTheDocument()
    await userEvent.type(await domainInput(), '.org')
    const diff = within(await screen.findByRole('group', { name: DIFF_HEADING }))
    const keycloak = within(diff.getByRole('group', { name: KEYCLOAK_HOST.label }))
    expect(keycloak.getByText(FRONTEND_URL).closest('li')).toHaveTextContent(`https://keycloak.${OLD_BASE} → https://keycloak.${BASE}`)
  })

  it('flags a pinned leaf with the kept value and the value it would have become', async () => {
    const pinned: RenderResult = {
      tree: { leaves: [] },
      diff: {
        groups: [
          {
            foundation: 'host.keycloak',
            changes: [{ path: FRONTEND_URL, status: 'pinned', before: 'https://keycloak.old.org', after: `https://keycloak.${BASE}`, kept: 'https://idp.hand.org' }],
          },
        ],
      },
      handSet: [],
      findings: [],
    }
    renderFoundations(ON_FOUNDATIONS, { render: (body) => (body.profile.answers?.['domain.base'] ? pinned : { tree: { leaves: [] }, diff: { groups: [] }, handSet: [], findings: [] }) })
    await userEvent.type(await domainInput(), BASE)
    const item = (await screen.findByText(PINNED_LABEL)).closest('li')
    expect(item).toHaveTextContent(FRONTEND_URL)
    expect(item).toHaveTextContent(`keeps https://idp.hand.org, would have become https://keycloak.${BASE}`)
  })

  describe('findings', () => {
    const answered: AppState = { ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values: { 'domain.base': BASE } } }
    const ON_TARGET = { ...DOMAIN_WARNING, rule: 'on-target', stage: StageId.Target, field: 'target.environment', message: 'Not on this stage.' }

    it('shows a finding under its field and keeps Next enabled', async () => {
      renderFoundations(answered, { render: renderWithFindings([DOMAIN_WARNING, ON_TARGET]) })
      const note = (await screen.findByText(new RegExp(DOMAIN_WARNING.message))).closest('p') as HTMLElement
      expect(note).toHaveTextContent(`Warning: ${DOMAIN_WARNING.message}`)
      expect(within(note).getByRole('link', { name: READ_MORE })).toHaveAttribute('href', repositoryDocUrl(DOMAIN_WARNING.doc))
      expect(await domainInput()).toHaveAccessibleDescription(expect.stringContaining(DOMAIN_WARNING.message))
      expect(screen.queryByText(ON_TARGET.message)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^Next/ })).toBeEnabled()
    })

    it('drops the finding once a render no longer reports it', async () => {
      const warnOnBase = renderWithFindings([DOMAIN_WARNING])
      renderFoundations(answered, { render: (body) => ({ ...warnOnBase(body), findings: body.profile.answers?.['domain.base'] === BASE ? [DOMAIN_WARNING] : [] }) })
      await screen.findByText(new RegExp(DOMAIN_WARNING.message))
      await userEvent.type(await domainInput(), '.eu')
      await waitFor(() => expect(screen.queryByText(new RegExp(DOMAIN_WARNING.message))).not.toBeInTheDocument())
    })
  })

  describe('DID', () => {
    const withDID: FoundationFakes = { foundations: () => DID_FOUNDATIONS }
    const HAND_DID = 'did:web:hand.example.org'
    const methodGroup = () => screen.findByRole('group', { name: DID_METHOD.label })
    const didInput = () => screen.getByRole('textbox', { name: DID_VALUE.label })

    it('asks the method and the DID in the Organization panel, not among the hosts', async () => {
      renderFoundations(ON_FOUNDATIONS, withDID)
      await methodGroup()
      const organization = within(panel('Organization'))
      expect(organization.getByRole('group', { name: DID_METHOD.label })).toBeInTheDocument()
      expect(organization.getByLabelText(DID_VALUE.label)).toHaveTextContent(DERIVED_PENDING)
      expect(within(screen.getByRole('group', { name: /Host names/ })).queryByText(DID_VALUE.label)).not.toBeInTheDocument()
    })

    it('shows each method with its note and checks the default while none is given', async () => {
      renderFoundations(ON_FOUNDATIONS, withDID)
      const group = within(await methodGroup())
      for (const o of DID_METHOD.options) {
        expect(group.getByRole('radio', { name: o.label })).toHaveAccessibleDescription(o.note)
      }
      expect(group.getByRole('radio', { name: 'did:web' })).toBeChecked()
    })

    it('derives the DID from the base domain with where it is used', async () => {
      renderFoundations(ON_FOUNDATIONS, withDID)
      await userEvent.type(await domainInput(), BASE)
      const organization = within(panel('Organization'))
      expect(organization.getByLabelText(DID_VALUE.label)).toHaveTextContent(`did:web:${BASE}`)
      const used = organization.getByRole('button', { name: usedByText(DID_VALUE) })
      expect(used).toHaveTextContent('used by 3 values in 3 components')
      await userEvent.click(used)
      const list = screen.getByRole('list', { name: `Where ${DID_VALUE.label} is used` })
      expect(within(list).getAllByRole('listitem').map((li) => li.textContent)).toEqual(DID_VALUE.targets.map((t) => t.path))
    })

    it('edits the DID from its derived value and resets it to derived', async () => {
      renderFoundations(ON_FOUNDATIONS, withDID)
      await userEvent.type(await domainInput(), BASE)
      await userEvent.click(screen.getByRole('button', { name: `${EDIT_LABEL} ${DID_VALUE.label}` }))
      expect(didInput()).toHaveValue(`did:web:${BASE}`)
      expect(didInput()).toHaveFocus()
      await userEvent.clear(didInput())
      await userEvent.type(didInput(), HAND_DID)
      expect(didInput()).toHaveValue(HAND_DID)
      await userEvent.click(screen.getByRole('button', { name: RESET_LABEL }))
      expect(screen.queryByRole('textbox', { name: DID_VALUE.label })).not.toBeInTheDocument()
      expect(within(panel('Organization')).getByLabelText(DID_VALUE.label)).toHaveTextContent(`did:web:${BASE}`)
    })

    it('drops a DID the new method does not match and asks for one under did:key', async () => {
      const handSet: AppState = { ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values: { 'domain.base': BASE, 'did.value': HAND_DID } } }
      renderFoundations(handSet, withDID)
      const group = within(await methodGroup())
      expect(didInput()).toHaveValue(HAND_DID)
      await userEvent.click(group.getByRole('radio', { name: 'did:key' }))
      expect(didInput()).toHaveValue('')
      expect(screen.queryByRole('button', { name: RESET_LABEL })).not.toBeInTheDocument()
      await userEvent.click(group.getByRole('radio', { name: 'did:web' }))
      expect(within(panel('Organization')).getByLabelText(DID_VALUE.label)).toHaveTextContent(`did:web:${BASE}`)
    })

    it('ends an edit when the method switches away, so switching back shows the derived DID', async () => {
      renderFoundations(ON_FOUNDATIONS, withDID)
      await userEvent.type(await domainInput(), BASE)
      await userEvent.click(screen.getByRole('button', { name: `${EDIT_LABEL} ${DID_VALUE.label}` }))
      const group = within(screen.getByRole('group', { name: DID_METHOD.label }))
      await userEvent.click(group.getByRole('radio', { name: 'did:key' }))
      await userEvent.click(group.getByRole('radio', { name: 'did:web' }))
      expect(screen.queryByRole('textbox', { name: DID_VALUE.label })).not.toBeInTheDocument()
      expect(within(panel('Organization')).getByLabelText(DID_VALUE.label)).toHaveTextContent(`did:web:${BASE}`)
    })

    it('shows a finding on the method under the method', async () => {
      const keyInProduction = { ...DOMAIN_WARNING, rule: 'production-did-key', field: 'answers.did.method', message: 'did:key in production.' }
      const answered: AppState = { ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values: { 'domain.base': BASE } } }
      renderFoundations(answered, { ...withDID, render: renderWithFindings([keyInProduction]) })
      const group = within(await methodGroup())
      expect(await group.findByText(new RegExp(keyInProduction.message))).toBeInTheDocument()
      expect(screen.getByRole('group', { name: DID_METHOD.label })).toHaveAccessibleDescription(expect.stringContaining(keyInProduction.message))
    })
  })
})
