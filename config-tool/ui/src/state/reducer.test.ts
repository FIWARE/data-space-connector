import { describe, expect, it } from 'vitest'
import { VersionSource } from '../api/load'
import { REPORT } from '../test/api'
import { emptyAnswers, Environment, TargetType } from './answers'
import { initialState, reducer, type AppState } from './reducer'
import { StageId } from './stageIds'

const KUBECONFIG = '/home/me/.kube/config'
const FILE = new File(['did:\n  enabled: true\n'], 'provider.yaml')

/** A session in the middle of a configuration: several stages seen, a context picked, answers given. */
const BUSY: AppState = {
  ...initialState,
  entered: true,
  stage: StageId.Features,
  visited: [StageId.Target, StageId.Role, StageId.Features],
  answers: { target: { ...emptyAnswers().target, type: TargetType.Cluster, namespace: 'provider', chartVersion: '10.15.0', environment: Environment.Production } },
  session: { kubeconfig: KUBECONFIG, chosenContext: 'prod' },
  expert: true,
}

describe('reducer', () => {
  it('keeps the kubeconfig path but drops the context pick on a new configuration', () => {
    const state = { ...initialState, session: { kubeconfig: KUBECONFIG, chosenContext: 'prod' } }
    expect(reducer(state, { type: 'newConfiguration' }).session).toEqual({ kubeconfig: KUBECONFIG, chosenContext: undefined })
  })

  it('starts over on Review when a file is loaded', () => {
    const next = reducer(BUSY, { type: 'fileLoaded', report: REPORT, file: FILE })
    expect(next).toMatchObject({ entered: true, stage: StageId.Review, visited: [StageId.Review], expert: false, loaded: REPORT, loadedFile: FILE })
    expect(next.session).toEqual({ kubeconfig: KUBECONFIG, chosenContext: undefined })
    expect(next.answers).toEqual({ target: { ...emptyAnswers().target, chartVersion: REPORT.chartVersion } })
  })

  it.each([
    [VersionSource.Header, REPORT.chartVersion],
    [VersionSource.Latest, ''],
  ])('takes the chart version only from a header (%s)', (chartVersionFrom, expected) => {
    const report = { ...REPORT, chartVersionFrom }
    expect(reducer(initialState, { type: 'fileLoaded', report, file: FILE }).answers.target.chartVersion).toBe(expected)
  })

  it('forgets the loaded file on a new configuration', () => {
    const loaded = reducer(BUSY, { type: 'fileLoaded', report: REPORT, file: FILE })
    expect(reducer(loaded, { type: 'newConfiguration' })).toMatchObject({ loaded: undefined, loadedFile: undefined })
  })

  it('swaps only the report when the same file is re-read under another root', () => {
    const loaded = { ...BUSY, stage: StageId.Review, loaded: REPORT, loadedFile: FILE }
    const rerooted = { ...REPORT, root: 'dsc' }
    expect(reducer(loaded, { type: 'rootChanged', report: rerooted, file: FILE })).toEqual({ ...loaded, loaded: rerooted })
  })

  it.each([
    ['another file was opened since', new File(['did: {}\n'], FILE.name)],
    ['the configuration was started over', undefined],
  ])('drops a re-read of a file that is no longer loaded (%s)', (_, loadedFile) => {
    const state = { ...BUSY, loaded: REPORT, loadedFile, rereading: true }
    expect(reducer(state, { type: 'rootChanged', report: { ...REPORT, root: 'dsc' }, file: FILE })).toEqual({ ...state, rereading: false })
  })

  it.each([
    ['a re-read answers', { type: 'rootChanged', report: REPORT, file: FILE } as const],
    ['a re-read fails', { type: 'rootRereadFailed' } as const],
  ])('marks a re-read in flight until %s', (_, end) => {
    const started = reducer({ ...BUSY, loaded: REPORT, loadedFile: FILE }, { type: 'rootRereadStarted' })
    expect(started.rereading).toBe(true)
    expect(reducer(started, end).rereading).toBe(false)
  })

  it('closes the Expert view on any stage jump', () => {
    const expert = reducer({ ...initialState, entered: true, loaded: REPORT }, { type: 'openExpert' })
    expect(expert.expert).toBe(true)
    expect(reducer(expert, { type: 'goTo', stage: StageId.Role }).expert).toBe(false)
  })

  it.each<[string, AppState['open'], AppState['open']]>([
    ['marks a field, keeping the ids sorted', ['target.type'], ['target.namespace', 'target.type']],
    ['unmarks a marked field', ['target.namespace', 'target.type'], ['target.type']],
  ])('%s and keeps its value', (_name, open, want) => {
    const next = reducer({ ...BUSY, open }, { type: 'toggleOpen', field: 'target.namespace' })
    expect(next.open).toEqual(want)
    expect(next.answers).toBe(BUSY.answers)
  })

  it('jumps to the stage of a focused field, closing the Expert view, until the stage is done with it', () => {
    const focused = reducer(BUSY, { type: 'focusField', field: 'target.namespace' })
    expect(focused).toMatchObject({ stage: StageId.Target, expert: false, focus: 'target.namespace' })
    expect(focused.visited).toEqual(BUSY.visited)
    expect(reducer(focused, { type: 'focusDone' }).focus).toBeUndefined()
  })

  it.each([
    ['a new configuration', { type: 'newConfiguration' } as const],
    ['a loaded file', { type: 'fileLoaded', report: REPORT, file: FILE } as const],
  ])('drops markers and focus on %s', (_name, action) => {
    const next = reducer({ ...BUSY, open: ['target.namespace'], focus: 'target.namespace' }, action)
    expect(next.open).toEqual([])
    expect(next.focus).toBeUndefined()
  })
})
