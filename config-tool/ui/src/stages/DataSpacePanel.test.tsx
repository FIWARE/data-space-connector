import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Finding } from '../api/render'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { AUTO_REGISTRATION_ID, CREDENTIAL_TYPES_ID, TIR_ADDRESS_ID } from '../state/dataspace'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import {
  AUTO_REGISTRATION,
  CREDENTIAL_TYPES,
  DATASPACE_FOUNDATIONS,
  DESCRIPTOR_PATH,
  IMPORTED,
  importPreview,
  mockApi,
  refusedDataspace,
  renderWithFindings,
  TIR_ADDRESS,
  TIR_STATUS,
  type DataspaceFakes,
  type FoundationFakes,
  type ImportBody,
} from '../test/api'
import {
  APPLY_LABEL,
  CHECK_LABEL,
  DATASPACE_PANEL,
  DISCARD_LABEL,
  IMPORT_BUTTON,
  IMPORT_LABEL,
  OUTCOME_ICONS,
  PREVIEW_LABEL,
  rowLabels,
  TIR_REVIEW_HEADING,
} from './dataSpaceText'

const TIR = 'https://tir.example.org/'
const BASE = 'https://dataspace.example.org'
const HTTP_BAD_GATEWAY = 502
const EMPTY_MESSAGE = 'This data space does not publish a TIR address, credential types or a registration mode yet. Enter them below.'

const TIR_REQUIRED: Finding = {
  rule: 'tir-address-for-verification',
  severity: 'error',
  stage: 'foundations',
  field: `answers.${TIR_ADDRESS_ID}`,
  message: 'The verifier cannot check issuers without the data space’s Trust Anchor.',
  doc: 'doc/deployment-integration/roles/provider/README.md#vcverifier',
}

const ON_FOUNDATIONS: AppState = {
  ...initialState,
  entered: true,
  stage: StageId.Foundations,
  visited: [StageId.Target, StageId.Role, StageId.Foundations],
  answers: { ...initialState.answers, role: 'provider' },
}

const withValues = (values: AppState['answers']['values']): AppState => ({ ...ON_FOUNDATIONS, answers: { ...ON_FOUNDATIONS.answers, values } })

/** Dataspace fakes that fail the test's expectation whenever a dataspace route is reached. */
const NO_CALLS: DataspaceFakes = {
  check: () => {
    throw new Error('unexpected TIR check')
  },
  import: () => {
    throw new Error('unexpected import')
  },
}

function renderPanel(state: AppState = ON_FOUNDATIONS, dataspace: DataspaceFakes = {}, foundations: FoundationFakes = {}) {
  const fetchMock = mockApi(undefined, undefined, undefined, {}, {}, {}, { foundations: () => DATASPACE_FOUNDATIONS, ...foundations }, dataspace)
  render(
    <StoreProvider initial={state}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
  return fetchMock
}

const dataspaceCalls = (fetchMock: ReturnType<typeof mockApi>) =>
  fetchMock.mock.calls.filter(([input]) => new URL(String(input), 'http://localhost').pathname.startsWith('/api/dataspace/'))
const panel = () => screen.findByRole('group', { name: DATASPACE_PANEL })
const tirInput = () => screen.findByRole('textbox', { name: TIR_ADDRESS.label })
const typesGroup = () => screen.getByRole('group', { name: CREDENTIAL_TYPES.label })
const typeValues = () => within(typesGroup()).getAllByRole('textbox', { name: /^Type \d+$/ }).map((i) => (i as HTMLInputElement).value)
const goToReview = () => userEvent.click(within(screen.getByRole('navigation', { name: 'Stages' })).getByRole('button', { name: /Review/ }))

async function importFrom(url = BASE) {
  await userEvent.type(screen.getByRole('textbox', { name: IMPORT_LABEL }), url)
  await userEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }))
  return screen.findByRole('group', { name: PREVIEW_LABEL })
}

describe('DataSpacePanel', () => {
  it('sits between Organization and Network', async () => {
    renderPanel()
    await panel()
    const headings = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)
    expect(headings.slice(0, 3)).toEqual(['Organization', DATASPACE_PANEL, 'Network'])
  })

  it('makes no dataspace call while the fields are edited', async () => {
    const fetchMock = renderPanel(ON_FOUNDATIONS, NO_CALLS)
    await userEvent.type(await tirInput(), TIR)
    await userEvent.type(screen.getByRole('textbox', { name: IMPORT_LABEL }), BASE)
    await userEvent.click(screen.getByRole('button', { name: /add a credential type/ }))
    await userEvent.type(screen.getByRole('textbox', { name: rowLabels(1).type }), 'MembershipCredential')
    await userEvent.click(screen.getByRole('checkbox', { name: AUTO_REGISTRATION.label }))
    expect(dataspaceCalls(fetchMock)).toEqual([])
  })

  it.each<[string, DataspaceFakes['check'], string, string]>([
    ['reachable', () => ({ status: 'reachable', httpStatus: TIR_STATUS }), OUTCOME_ICONS.reachable, `reachable (HTTP ${TIR_STATUS})`],
    ['unreachable', () => ({ status: 'unreachable', error: 'connection refused' }), OUTCOME_ICONS.unreachable, 'unreachable: connection refused'],
  ])('checks the TIR once on Check and shows it %s as icon and text', async (_name, check, icon, text) => {
    const fetchMock = renderPanel(withValues({ [TIR_ADDRESS_ID]: TIR }), { check })
    await userEvent.click(within(await panel()).getByRole('button', { name: CHECK_LABEL }))
    const status = within(await panel()).getByRole('status')
    await waitFor(() => expect(status).toHaveTextContent(text))
    expect(status).toHaveTextContent(icon)
    const [only, ...rest] = dataspaceCalls(fetchMock)
    expect(rest).toEqual([])
    expect(JSON.parse(String((only?.[1] as RequestInit).body))).toEqual({ url: TIR })
  })

  it('shows a refused check as an error and records nothing', async () => {
    renderPanel(withValues({ [TIR_ADDRESS_ID]: TIR }), { check: () => HTTP_BAD_GATEWAY })
    await userEvent.click(within(await panel()).getByRole('button', { name: CHECK_LABEL }))
    expect(await within(await panel()).findByText(refusedDataspace(HTTP_BAD_GATEWAY))).toBeInTheDocument()
  })

  it.each<[string, DataspaceFakes, string]>([
    ['checked', {}, 'reachable'],
    ['refused', { check: () => HTTP_BAD_GATEWAY }, refusedDataspace(HTTP_BAD_GATEWAY)],
  ])('resets a %s outcome to not checked once the address changes', async (_name, fakes, before) => {
    renderPanel(withValues({ [TIR_ADDRESS_ID]: TIR }), fakes)
    await userEvent.click(within(await panel()).getByRole('button', { name: CHECK_LABEL }))
    const status = within(await panel()).getByRole('status')
    await waitFor(() => expect(status).toHaveTextContent(before))
    await userEvent.type(await tirInput(), 'x')
    expect(status).toHaveTextContent('not checked')
    expect(status).not.toHaveTextContent(before)
  })

  it('lists the outcome in Review as information', async () => {
    renderPanel(withValues({ [TIR_ADDRESS_ID]: TIR }))
    await userEvent.click(within(await panel()).getByRole('button', { name: CHECK_LABEL }))
    await waitFor(() => expect(within(screen.getByRole('group', { name: DATASPACE_PANEL })).getByRole('status')).toHaveTextContent('reachable'))
    await goToReview()
    const line = screen.getByRole('group', { name: TIR_REVIEW_HEADING })
    expect(line).toHaveTextContent(TIR)
    expect(line).toHaveTextContent(`reachable (HTTP ${TIR_STATUS})`)
    expect(screen.queryByRole('heading', { name: 'Findings' })).not.toBeInTheDocument()
  })

  it('previews an import and sets the answers only on Apply', async () => {
    let body: ImportBody | undefined
    const record = (b: ImportBody) => {
      body = b
      return { url: `${BASE}${DESCRIPTOR_PATH}`, preview: importPreview(b.profile) }
    }
    renderPanel(ON_FOUNDATIONS, { import: record })
    await panel()
    const preview = within(await importFrom())
    expect(body?.url).toBe(BASE)
    expect(preview.getByText(/Trust Anchor TIR address: — → https:\/\/tir.dataspace.org\//)).toBeInTheDocument()
    expect(preview.getByText(/Not used: contact/)).toBeInTheDocument()
    expect(await tirInput()).toHaveValue('')
    await userEvent.click(preview.getByRole('button', { name: APPLY_LABEL }))
    expect(screen.queryByRole('group', { name: PREVIEW_LABEL })).not.toBeInTheDocument()
    expect(await tirInput()).toHaveValue(String(IMPORTED[TIR_ADDRESS_ID]))
    expect(typeValues()).toEqual(['MembershipCredential', 'UserCredential'])
    expect(screen.getByRole('checkbox', { name: AUTO_REGISTRATION.label })).toBeChecked()
  })

  it('changes nothing on Discard', async () => {
    renderPanel(withValues({ [TIR_ADDRESS_ID]: TIR }))
    await panel()
    const preview = within(await importFrom())
    await userEvent.click(preview.getByRole('button', { name: DISCARD_LABEL }))
    expect(screen.queryByRole('group', { name: PREVIEW_LABEL })).not.toBeInTheDocument()
    expect(await tirInput()).toHaveValue(TIR)
    expect(within(typesGroup()).queryAllByRole('textbox')).toEqual([])
    expect(screen.getByRole('checkbox', { name: AUTO_REGISTRATION.label })).not.toBeChecked()
  })

  it('shows an empty preview as information with the unused keys', async () => {
    renderPanel(ON_FOUNDATIONS, { import: () => ({ url: BASE, preview: { changes: [], problems: [], unused: ['supported_models'], message: EMPTY_MESSAGE } }) })
    await panel()
    const preview = await importFrom()
    const info = within(preview).getByText(new RegExp(EMPTY_MESSAGE))
    expect(info).toHaveClass('info')
    expect(info).not.toHaveClass('field-error')
    expect(preview).toHaveTextContent('Not used: supported_models')
    expect(within(preview).getByRole('button', { name: APPLY_LABEL })).toBeDisabled()
  })

  it('shows a failed import in the live region', async () => {
    renderPanel(ON_FOUNDATIONS, { import: () => HTTP_BAD_GATEWAY })
    await panel()
    await userEvent.type(screen.getByRole('textbox', { name: IMPORT_LABEL }), BASE)
    await userEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }))
    const error = await screen.findByText(refusedDataspace(HTTP_BAD_GATEWAY))
    expect(error.closest('[aria-live]')).not.toBeNull()
  })

  it('adds, edits, reorders and removes credential types', async () => {
    renderPanel()
    await panel()
    const add = () => userEvent.click(within(typesGroup()).getByRole('button', { name: /add a credential type/ }))
    await add()
    expect(screen.getByRole('textbox', { name: rowLabels(1).type })).toHaveFocus()
    await userEvent.keyboard('A')
    await add()
    await userEvent.keyboard('B')
    await userEvent.type(screen.getByRole('textbox', { name: rowLabels(2).description }), 'second')
    expect(typeValues()).toEqual(['A', 'B'])
    expect(screen.getByRole('button', { name: rowLabels(1).up })).toBeDisabled()
    expect(screen.getByRole('button', { name: rowLabels(2).down })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: rowLabels(2).up }))
    expect(typeValues()).toEqual(['B', 'A'])
    expect(screen.getByRole('textbox', { name: rowLabels(1).description })).toHaveValue('second')
    await userEvent.click(screen.getByRole('button', { name: rowLabels(1).down }))
    expect(typeValues()).toEqual(['A', 'B'])
    await userEvent.click(screen.getByRole('button', { name: rowLabels(1).remove }))
    expect(typeValues()).toEqual(['B'])
  })

  it('names the failing row by its label and marks its input', async () => {
    renderPanel(withValues({ [CREDENTIAL_TYPES_ID]: [{ type: 'A' }, { type: 'A' }] }))
    await panel()
    const error = within(typesGroup()).getByText(`${rowLabels(2).type} repeats an earlier type`)
    const second = screen.getByRole('textbox', { name: rowLabels(2).type })
    expect(second).toHaveAttribute('aria-invalid', 'true')
    expect(second).toHaveAttribute('aria-describedby', error.id)
    expect(screen.getByRole('textbox', { name: rowLabels(1).type })).not.toHaveAttribute('aria-invalid')
    expect(screen.getByRole('textbox', { name: rowLabels(2).description })).not.toHaveAttribute('aria-invalid')
  })

  it.each([TIR_ADDRESS, CREDENTIAL_TYPES, AUTO_REGISTRATION])('marks $label to ask later and links it back from Review', async (f) => {
    renderPanel()
    await panel()
    await userEvent.click(screen.getByRole('button', { name: `Ask ${f.label} later` }))
    await goToReview()
    const open = screen.getByRole('heading', { name: /Open questions/ }).closest('div') as HTMLElement
    await userEvent.click(within(open).getByRole('button', { name: `Go to ${f.label}` }))
    await waitFor(() => expect(document.getElementById(`field-answers.${f.id}`)).toHaveFocus())
  })

  it('shows the TIR finding as an error on the field and in Review', async () => {
    const values = { 'domain.base': 'example.org' }
    renderPanel(withValues(values), {}, { render: renderWithFindings([TIR_REQUIRED]) })
    const note = (await screen.findByText(new RegExp(TIR_REQUIRED.message))).closest('p') as HTMLElement
    expect(note).toHaveTextContent(`Error: ${TIR_REQUIRED.message}`)
    expect(await tirInput()).toHaveAccessibleDescription(expect.stringContaining(TIR_REQUIRED.message))
    await goToReview()
    const findings = (await screen.findByRole('heading', { name: 'Findings' })).closest('div') as HTMLElement
    const item = within(findings).getByText(TIR_REQUIRED.message).closest('li') as HTMLElement
    expect(item).toHaveClass('error')
    expect(item).toHaveTextContent('Error')
    expect(within(item).getByRole('button', { name: `Go to ${TIR_ADDRESS.label}` })).toBeInTheDocument()
  })

  it.each([CREDENTIAL_TYPES_ID, AUTO_REGISTRATION_ID])('leaves %s out when the bundle does not declare it', async (id) => {
    renderPanel(ON_FOUNDATIONS, {}, { foundations: () => ({ foundations: DATASPACE_FOUNDATIONS.foundations.filter((f) => f.id !== id) }) })
    await tirInput()
    const label = DATASPACE_FOUNDATIONS.foundations.find((f) => f.id === id)?.label ?? id
    expect(screen.queryByText(label)).not.toBeInTheDocument()
  })
})
