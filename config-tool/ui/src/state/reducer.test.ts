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
    const state = { ...BUSY, loaded: REPORT, loadedFile }
    expect(reducer(state, { type: 'rootChanged', report: { ...REPORT, root: 'dsc' }, file: FILE })).toBe(state)
  })

  it('closes the Expert view on any stage jump', () => {
    const expert = reducer({ ...initialState, entered: true, loaded: REPORT }, { type: 'openExpert' })
    expect(expert.expert).toBe(true)
    expect(reducer(expert, { type: 'goTo', stage: StageId.Role }).expert).toBe(false)
  })
})
