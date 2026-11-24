import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { ProfileResponse } from '../api/profile'
import { STAGES } from '../stages'
import { emptyAnswers, TargetType } from '../state/answers'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { DEFAULT_DIR, mockApi, REPORT, refusedProfile, type SaveBody } from '../test/api'
import { Chrome } from './Chrome'
import { SAVE_PATH_LABEL } from './SaveDialog'

const HTTP_CONFLICT = 409
const HTTP_UNPROCESSABLE = 422
const KNOWN_PATH = '/srv/dsc/provider.dscprofile.yaml'
const NOTE = 'overrides will not be restored on reopen: provider.yaml is not next to the profile'
const ENTERED: AppState = { ...initialState, entered: true, visited: [initialState.stage] }

type Save = (body: SaveBody) => ProfileResponse | number

function echo(notes: string[] = []): Save {
  return (body) => ({ path: body.path, profile: body.profile, notes })
}

function renderChrome(save: Save, state: AppState = ENTERED) {
  const saves: SaveBody[] = []
  mockApi(undefined, undefined, undefined, {
    save: (body) => {
      saves.push(body)
      return save(body)
    },
  })
  render(
    <StoreProvider initial={state}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
  return saves
}

const railSave = () => within(screen.getByRole('navigation', { name: 'Stages' })).getByRole('button', { name: /Save profile/ })
const dialog = () => screen.findByRole('dialog', { name: 'Save profile' })

describe('SaveDialog', () => {
  it.each([
    ['', `${DEFAULT_DIR}/connector.dscprofile.yaml`],
    ['provider', `${DEFAULT_DIR}/provider.dscprofile.yaml`],
  ])('proposes a path in the default directory for release %j', async (releaseName, expected) => {
    renderChrome(echo(), { ...ENTERED, answers: { values: {}, features: {}, target: { ...emptyAnswers().target, releaseName } } })
    await userEvent.click(railSave())
    expect(within(await dialog()).getByLabelText(SAVE_PATH_LABEL)).toHaveValue(expected)
  })

  it('saves with Ctrl+S, closes and names the profile in the title', async () => {
    const saves = renderChrome(echo())
    await userEvent.keyboard('{Control>}s{/Control}')
    await userEvent.click(within(await dialog()).getByRole('button', { name: 'Save' }))
    expect(saves).toMatchObject([{ path: `${DEFAULT_DIR}/connector.dscprofile.yaml`, overwrite: false }])
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByRole('banner')).toHaveTextContent('connector')
  })

  it('reports the save in the rail until an answer changes', async () => {
    renderChrome(echo(), { ...ENTERED, profilePath: KNOWN_PATH })
    await userEvent.click(railSave())
    const rail = screen.getByRole('navigation', { name: 'Stages' })
    expect(await within(rail).findByRole('status')).toHaveTextContent('Saved provider')
    await userEvent.type(screen.getByLabelText('Namespace'), 'x')
    expect(within(rail).queryByRole('status')).toBeNull()
  })

  it('asks before replacing an existing file and replaces it on confirm', async () => {
    const saves = renderChrome((body) => (body.overwrite ? echo()(body) : HTTP_CONFLICT))
    await userEvent.click(railSave())
    const form = await dialog()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(within(form).getByRole('alert')).toHaveTextContent('exists. Replace it?')
    await userEvent.click(within(form).getByRole('button', { name: 'Replace' }))
    expect(saves.map((body) => body.overwrite)).toEqual([false, true])
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('shows the note of a save that will not find its overrides', async () => {
    renderChrome(echo([NOTE]), { ...ENTERED, loaded: REPORT })
    await userEvent.click(railSave())
    const form = await dialog()
    expect(form).toHaveTextContent(`Save next to ${REPORT.source} so reopening finds its overrides.`)
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(within(form).getByRole('status')).toHaveTextContent(NOTE)
  })

  it('keeps the dialog and the typed path when the server refuses the profile', async () => {
    renderChrome(() => HTTP_UNPROCESSABLE)
    await userEvent.click(railSave())
    const form = await dialog()
    const field = within(form).getByLabelText(SAVE_PATH_LABEL)
    await userEvent.clear(field)
    await userEvent.type(field, '/tmp/x.dscprofile.yaml{Enter}')
    expect(await within(form).findByRole('alert')).toHaveTextContent(refusedProfile('/tmp/x.dscprofile.yaml', HTTP_UNPROCESSABLE))
    expect(field).toHaveValue('/tmp/x.dscprofile.yaml')
  })

  it('saves a known profile in place, and opens the dialog with the error when that fails', async () => {
    let fail = false
    const saves = renderChrome((body) => (fail ? HTTP_UNPROCESSABLE : echo()(body)), { ...ENTERED, profilePath: KNOWN_PATH })
    await userEvent.click(railSave())
    expect(saves).toMatchObject([{ path: KNOWN_PATH, overwrite: true }])
    expect(screen.queryByRole('dialog')).toBeNull()
    fail = true
    await userEvent.click(railSave())
    const form = await dialog()
    expect(within(form).getByLabelText(SAVE_PATH_LABEL)).toHaveValue(KNOWN_PATH)
    expect(within(form).getByRole('alert')).toHaveTextContent(refusedProfile(KNOWN_PATH, HTTP_UNPROCESSABLE))
  })

  it('shows the notes of an in-place save on Review', async () => {
    renderChrome(echo([NOTE]), { ...ENTERED, stage: StageId.Review, profilePath: KNOWN_PATH })
    await userEvent.click(screen.getByRole('button', { name: 'Save profile' }))
    expect(await screen.findByText(NOTE)).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('keeps Save enabled while Target shows a format error', async () => {
    const saves = renderChrome(echo(), { ...ENTERED, answers: { values: {}, features: {}, target: { ...emptyAnswers().target, type: TargetType.Cluster, namespace: 'Prod_NS' } } })
    expect(screen.getByText(/must consist of lowercase/)).toBeInTheDocument()
    expect(railSave()).toBeEnabled()
    await userEvent.click(railSave())
    await userEvent.click(within(await dialog()).getByRole('button', { name: 'Save' }))
    expect(saves[0]?.profile.target.namespace).toBe('Prod_NS')
  })
})
