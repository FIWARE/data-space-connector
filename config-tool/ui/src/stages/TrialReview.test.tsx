import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Chrome } from '../chrome/Chrome'
import { TRIAL_SAVE_DISABLED } from '../chrome/SaveDialog'
import { STAGES } from '../stages'
import { initialState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { DEFAULT_DIR, mockApi, refusedTrial, TRIAL, type TrialFakes } from '../test/api'
import { NOTHING_WRITTEN, TRIAL_DIR_LABEL, WRITE_LABEL } from './TrialReview'

const HTTP_CONFLICT = 409
const HTTP_UNPROCESSABLE = 422
const OTHER_DIR = '/tmp/trial'

function renderTrial(trial: TrialFakes = {}) {
  const fetchMock = mockApi(undefined, undefined, undefined, {}, trial)
  render(
    <StoreProvider initial={{ ...initialState, entered: true, stage: StageId.Review, visited: [StageId.Review], trial: TRIAL }}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
  return fetchMock
}

const panel = (name: RegExp) => within(screen.getByRole('heading', { level: 3, name }).closest('.panel') as HTMLElement)

async function writeTo(dir: string) {
  const field = screen.getByLabelText(TRIAL_DIR_LABEL)
  await waitFor(() => expect(field).toHaveValue(DEFAULT_DIR))
  await userEvent.clear(field)
  await userEvent.type(field, dir)
  await userEvent.click(screen.getByRole('button', { name: WRITE_LABEL }))
}

describe('TrialReview', () => {
  it('heads the trial with the Development badge and links the Quick Start', () => {
    renderTrial()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Local trial Development')
    expect(screen.getByRole('link', { name: /Quick Start/ })).toHaveAttribute('href', expect.stringContaining('#quick-start'))
  })

  it('lists the three releases in install order with chart, file and source', () => {
    renderTrial()
    const releases = panel(/^Releases/).getAllByRole('listitem')
    expect(releases.map((li) => within(li).getByRole('heading', { level: 4 }).textContent)).toEqual(['trust-anchor', 'provider', 'consumer'])
    TRIAL.releases.forEach((release, index) => {
      const entry = within(releases[index] as HTMLElement)
      expect(entry.getByText(`${release.chart}@${release.chartVersion}`)).toBeInTheDocument()
      expect(entry.getByText(release.file)).toBeInTheDocument()
      expect(entry.getByText(release.source)).toBeInTheDocument()
      expect(entry.getByText(release.values.trim())).not.toBeVisible()
    })
  })

  it('shows every warning with its releases and no way to dismiss one', () => {
    renderTrial()
    const warnings = panel(/^Production warnings/)
    expect(screen.getByRole('heading', { level: 3, name: `Production warnings (${TRIAL.warnings.length})` })).toBeInTheDocument()
    expect(warnings.getAllByRole('listitem').map((li) => li.textContent)).toEqual(TRIAL.warnings.map((w) => expect.stringContaining(`${w.message} (${w.releases.join(', ')})`)))
    expect(warnings.queryByRole('button')).toBeNull()
  })

  it('disables saving a profile, naming the reason', () => {
    renderTrial()
    const save = within(screen.getByRole('main')).getByRole('button', { name: 'Save profile' })
    expect(save).toBeDisabled()
    expect(save).toHaveAttribute('title', TRIAL_SAVE_DISABLED)
  })

  it('writes the three files into the chosen directory and lists them', async () => {
    const fetchMock = renderTrial()
    await writeTo(OTHER_DIR)
    const status = await screen.findByRole('status')
    expect(within(status).getAllByRole('listitem').map((li) => li.textContent)).toEqual(TRIAL.releases.map((r) => `${OTHER_DIR}/${r.file}`))
    expect(fetchMock).toHaveBeenCalledWith('/api/trial/write', expect.objectContaining({ body: JSON.stringify({ dir: OTHER_DIR }) }))
  })

  it.each<[string, number, string]>([
    ['a conflict says nothing was written', HTTP_CONFLICT, `${refusedTrial(HTTP_CONFLICT)} ${NOTHING_WRITTEN}`],
    ['another refusal shows the message only', HTTP_UNPROCESSABLE, refusedTrial(HTTP_UNPROCESSABLE)],
  ])('%s', async (_name, status, expected) => {
    renderTrial({ write: () => status })
    await writeTo(OTHER_DIR)
    expect(await screen.findByRole('alert')).toHaveTextContent(expected)
    expect(screen.queryByRole('status')).toBeNull()
  })
})
