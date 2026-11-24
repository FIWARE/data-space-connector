import { describe, expect, it } from 'vitest'
import { VersionSource } from '../api/load'
import { toProfile, type Profile } from '../api/profile'
import type { RoleChange } from '../api/roles'
import { FOUNDATIONS, REPORT, TRIAL } from '../test/api'
import { emptyAnswers, Environment, TargetType } from './answers'
import { initialState, reducer, type Action, type AppState } from './reducer'
import { StageId } from './stageIds'

const KUBECONFIG = '/home/me/.kube/config'
const FILE = new File(['did:\n  enabled: true\n'], 'provider.yaml')

/** A session in the middle of a configuration: several stages seen, a context picked, answers given. */
const BUSY: AppState = {
  ...initialState,
  entered: true,
  stage: StageId.Features,
  visited: [StageId.Target, StageId.Role, StageId.Features],
  answers: { values: {}, features: {}, target: { ...emptyAnswers().target, type: TargetType.Cluster, namespace: 'provider', chartVersion: '10.15.0', environment: Environment.Production } },
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
    expect(next.answers).toEqual({ values: { 'domain.base': 'example.org' }, features: {}, target: { ...emptyAnswers().target, chartVersion: REPORT.chartVersion } })
  })

  it('seeds only the foundation answers a loaded file derives, and saving keeps them', () => {
    const next = reducer(BUSY, { type: 'fileLoaded', report: REPORT, file: FILE })
    expect(next.answers.values).toEqual({ 'domain.base': 'example.org' })
    expect(next.answers.features).toEqual({})
    expect(toProfile({ ...next, foundations: FOUNDATIONS.foundations }).answers).toEqual({ 'domain.base': 'example.org' })
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

const PROFILE_PATH = '/home/me/dsc/provider.dscprofile.yaml'
const SAVED_PROFILE: Profile = {
  format: 1,
  model: { chart: 'data-space-connector', version: '10.15.0', revision: 1 },
  target: BUSY.answers.target,
  answers: {},
  open: [{ field: 'target.namespace' }, { field: 'target.context' }],
  source: { file: REPORT.source, root: '', sha256: REPORT.sha256 },
  secrets: {},
}
const NOTES = ['provider.yaml changed since the profile was saved']

describe('reducer: profiles', () => {
  it('restores answers and markers exactly and lands on Review when a profile is opened', () => {
    const next = reducer(BUSY, { type: 'profileOpened', path: PROFILE_PATH, profile: SAVED_PROFILE, report: REPORT, notes: NOTES })
    expect(next).toMatchObject({ entered: true, stage: StageId.Review, visited: [StageId.Review], expert: false, loaded: REPORT, loadedFile: undefined })
    expect(next.answers).toEqual(BUSY.answers)
    expect(next.open).toEqual(['target.context', 'target.namespace'])
    expect(next).toMatchObject({ profilePath: PROFILE_PATH, profileModel: SAVED_PROFILE.model, profileNotes: NOTES })
    expect(next.session.chosenContext).toBeUndefined()
  })

  it('keeps the answers but records path and notes when a profile is saved', () => {
    const next = reducer(BUSY, { type: 'profileSaved', path: PROFILE_PATH, profile: SAVED_PROFILE, notes: NOTES })
    expect(next).toEqual({ ...BUSY, profilePath: PROFILE_PATH, profileModel: SAVED_PROFILE.model, profileNotes: NOTES })
  })

  const opened = reducer(BUSY, { type: 'profileOpened', path: PROFILE_PATH, profile: SAVED_PROFILE, notes: NOTES })
  const saved = reducer(BUSY, { type: 'profileSaved', path: PROFILE_PATH, profile: SAVED_PROFILE, notes: NOTES })
  it.each<[string, AppState, Action]>([
    ['opened', opened, { type: 'newConfiguration' }],
    ['opened', opened, { type: 'fileLoaded', report: REPORT, file: FILE }],
    ['saved', saved, { type: 'newConfiguration' }],
    ['saved', saved, { type: 'fileLoaded', report: REPORT, file: FILE }],
  ])('forgets the %s profile on %o, so the next save asks for a path', (_, state, action) => {
    const next = reducer(state, action)
    expect(next.profilePath).toBeUndefined()
    expect(next.profileModel).toBeUndefined()
    expect(next.profileNotes).toBeUndefined()
  })
})

describe('reducer: trial', () => {
  const loaded = reducer(BUSY, { type: 'fileLoaded', report: REPORT, file: FILE })

  it('lands on Review with only Review visited, a development target and the trial held', () => {
    const next = reducer({ ...loaded, profilePath: PROFILE_PATH }, { type: 'trialStarted', trial: TRIAL })
    expect(next).toMatchObject({ entered: true, stage: StageId.Review, visited: [StageId.Review], expert: false, open: [], trial: TRIAL })
    expect(next.answers).toEqual({ values: {}, features: {}, target: { ...emptyAnswers().target, environment: Environment.Development } })
    expect(next.loaded).toBeUndefined()
    expect(next.loadedFile).toBeUndefined()
    expect(next.profilePath).toBeUndefined()
    expect(next.session).toEqual({ kubeconfig: KUBECONFIG, chosenContext: undefined })
  })

  const trial = reducer(BUSY, { type: 'trialStarted', trial: TRIAL })
  it.each<Action>([
    { type: 'newConfiguration' },
    { type: 'fileLoaded', report: REPORT, file: FILE },
    { type: 'profileOpened', path: PROFILE_PATH, profile: SAVED_PROFILE, notes: [] },
  ])('drops the trial on $type', (action) => {
    expect(reducer(trial, action).trial).toBeUndefined()
  })
})

describe('reducer: roles', () => {
  const DSC = 'data-space-connector'
  const TRUST_ANCHOR = 'trust-anchor'
  const DSC_MODEL = { chart: DSC, version: '10.16.0', revision: 3 }
  const TA_MODEL = { chart: TRUST_ANCHOR, version: '1.3.6', revision: 1 }
  const FEATURES = { 'data.scorpio': { enabled: false, source: 'loaded' }, 'identity.issuer': { enabled: true, source: 'preset' } } as const
  const SWITCH = { fromChart: DSC, fromVersion: '10.15.0', toChart: TRUST_ANCHOR, droppedChoices: [], carriedAnswers: [], droppedAnswers: [] }
  const PROVIDER: Profile = { ...SAVED_PROFILE, model: DSC_MODEL, role: 'provider', features: FEATURES }
  const OPERATOR: Profile = {
    ...SAVED_PROFILE,
    model: TA_MODEL,
    catalogue: DSC_MODEL,
    target: { ...BUSY.answers.target, chart: TRUST_ANCHOR, chartVersion: '' },
    role: 'operator',
    features: { 'ta.registry': { enabled: true, source: 'preset' } },
  }
  const provider = reducer(BUSY, { type: 'roleApplied', profile: PROVIDER, chartSwitch: null, change: null })
  const operator = reducer(BUSY, { type: 'roleApplied', profile: OPERATOR, chartSwitch: SWITCH, change: null })

  it('takes role, features and model from a preset that keeps the chart, without a notice', () => {
    expect(provider.answers).toEqual({ ...BUSY.answers, role: 'provider', features: FEATURES })
    expect(provider).toMatchObject({ profileModel: DSC_MODEL, profileCatalogue: undefined, versionOpen: false, chartSwitch: undefined })
  })

  it('takes the switched chart, the cleared version and the catalogue from a preset that switches the chart', () => {
    expect(operator.answers.target).toEqual({ ...BUSY.answers.target, chart: TRUST_ANCHOR, chartVersion: '' })
    expect(operator.answers).toMatchObject({ role: 'operator', features: OPERATOR.features })
    expect(operator).toMatchObject({ profileModel: TA_MODEL, profileCatalogue: DSC_MODEL, versionOpen: true, chartSwitch: SWITCH })
  })

  it('drops the notices on a stage jump', () => {
    const cleared = reducer(provider, { type: 'setTarget', target: { chart: TRUST_ANCHOR, chartVersion: '' } })
    expect(cleared.roleCleared).toBeDefined()
    expect(reducer(operator, { type: 'goTo', stage: StageId.Target }).chartSwitch).toBeUndefined()
    expect(reducer(cleared, { type: 'goTo', stage: StageId.Role }).roleCleared).toBeUndefined()
  })

  it.each<[string, AppState, string]>([
    ['provider', provider, DSC],
    ['operator', operator, TRUST_ANCHOR],
  ])('clears the %s role, its features and catalogue when the chart changes, naming them in a notice', (role, state, fromChart) => {
    const toChart = fromChart === DSC ? TRUST_ANCHOR : DSC
    const next = reducer(state, { type: 'setTarget', target: { chart: toChart, chartVersion: '' } })
    expect(next.answers).toMatchObject({ role: undefined, features: {} })
    expect(next.profileCatalogue).toBeUndefined()
    expect(next.roleCleared).toEqual({ role, fromChart, toChart })
  })

  it('names no cleared role when none was picked', () => {
    expect(reducer(BUSY, { type: 'setTarget', target: { chart: TRUST_ANCHOR } }).roleCleared).toBeUndefined()
  })

  it.each<[string, Partial<AppState['answers']['target']>]>([
    ['the version', { chartVersion: '10.16.0' }],
    ['the same chart', { chart: TRUST_ANCHOR }],
    ['the namespace', { namespace: 'other' }],
  ])('keeps role, features and catalogue when %s is set', (_name, change) => {
    const next = reducer(operator, { type: 'setTarget', target: change })
    expect(next.answers).toMatchObject({ role: 'operator', features: OPERATOR.features })
    expect(next.profileCatalogue).toEqual(DSC_MODEL)
    expect(next.roleCleared).toBeUndefined()
  })

  it('keeps the catalogue the server returns on save', () => {
    expect(reducer(BUSY, { type: 'profileSaved', path: PROFILE_PATH, profile: OPERATOR, notes: [] }).profileCatalogue).toEqual(DSC_MODEL)
  })

  it('restores role, features and catalogue when a profile is opened, and forgets the catalogue on a new configuration', () => {
    const opened = reducer(BUSY, { type: 'profileOpened', path: PROFILE_PATH, profile: OPERATOR, notes: [] })
    expect(opened.answers).toMatchObject({ role: 'operator', features: OPERATOR.features })
    expect(opened.profileCatalogue).toEqual(DSC_MODEL)
    expect(reducer(opened, { type: 'newConfiguration' })).toMatchObject({ profileCatalogue: undefined, answers: emptyAnswers() })
  })

  it.each<[string, Profile, boolean]>([
    ['a role without a version', OPERATOR, true],
    ['a role with a version', PROVIDER, false],
    ['no role and no version', { ...SAVED_PROFILE, target: { ...BUSY.answers.target, chartVersion: '' } }, false],
  ])('opening a profile with %s leaves the version open: %s', (_name, profile, open) => {
    expect(reducer(BUSY, { type: 'profileOpened', path: PROFILE_PATH, profile, notes: [] }).versionOpen).toBe(open)
  })

  it.each<[string, Action]>([
    ['a chosen version', { type: 'setTarget', target: { chartVersion: '1.3.6' } }],
    ['a chart change', { type: 'setTarget', target: { chart: DSC, chartVersion: '' } }],
    ['a loaded file', { type: 'fileLoaded', report: REPORT, file: FILE }],
    ['the trial', { type: 'trialStarted', trial: TRIAL }],
    ['a new configuration', { type: 'newConfiguration' }],
  ])('stops leaving the version open on %s', (_name, action) => {
    expect(reducer(operator, action).versionOpen).toBe(false)
  })

  it.each<[string, Action]>([
    ['an unrelated answer', { type: 'setTarget', target: { namespace: 'other' } }],
    ['a preset that keeps the chart', { type: 'roleApplied', profile: OPERATOR, chartSwitch: null, change: null }],
  ])('keeps the version open on %s', (_name, action) => {
    expect(reducer(operator, action).versionOpen).toBe(true)
  })

  const CHANGE: RoleChange = { fromRole: 'provider', toRole: 'operator', enabled: ['ta.registry'], disabled: [], kept: [] }
  const changed = reducer(provider, { type: 'roleApplied', profile: OPERATOR, chartSwitch: SWITCH, change: CHANGE })

  it('records a role change with its chart switch, and nothing for the first pick', () => {
    expect(provider.roleChanges).toEqual([])
    expect(changed.roleChanges).toEqual([{ change: CHANGE, chartSwitch: SWITCH }])
    const back = { ...CHANGE, fromRole: 'operator', toRole: 'provider' }
    const again = reducer(changed, { type: 'roleApplied', profile: PROVIDER, chartSwitch: null, change: back })
    expect(again.roleChanges).toEqual([{ change: CHANGE, chartSwitch: SWITCH }, { change: back, chartSwitch: null }])
  })

  it.each<[string, Action]>([
    ['a new configuration', { type: 'newConfiguration' }],
    ['an opened profile', { type: 'profileOpened', path: PROFILE_PATH, profile: PROVIDER, notes: [] }],
    ['a loaded file', { type: 'fileLoaded', report: REPORT, file: FILE }],
    ['the trial', { type: 'trialStarted', trial: TRIAL }],
  ])('forgets the role changes on %s', (_name, action) => {
    expect(reducer(changed, action).roleChanges).toEqual([])
  })

  it.each<[string, Action]>([
    ['a chart change', { type: 'setTarget', target: { chart: DSC, chartVersion: '' } }],
    ['a stage jump', { type: 'goTo', stage: StageId.Target }],
    ['a save', { type: 'profileSaved', path: PROFILE_PATH, profile: OPERATOR, notes: [] }],
  ])('keeps the role changes on %s', (_name, action) => {
    expect(reducer(changed, action).roleChanges).toEqual(changed.roleChanges)
  })

  it('replaces the features on a toggle and keeps role, target and model, which makes the saved profile stale', () => {
    const toggled = { ...FEATURES, 'identity.issuer': { enabled: false, source: 'user' } } as const
    const next = reducer(provider, { type: 'featuresChanged', features: toggled })
    expect(next.answers).toEqual({ ...provider.answers, features: toggled })
    expect(next.answers).not.toBe(provider.answers)
    expect(next).toMatchObject({ stage: provider.stage, profileModel: DSC_MODEL, chartSwitch: undefined })
  })
})

describe('reducer: foundation answers', () => {
  const ANSWERED: AppState = {
    ...BUSY,
    answers: { ...BUSY.answers, target: { ...BUSY.answers.target, chart: 'data-space-connector' }, values: { 'domain.base': 'example.org', 'host.verifier': 'v.example.org' } },
    open: ['answers.host.verifier', 'target.namespace'],
    foundations: FOUNDATIONS.foundations,
  }

  it('drops the answers, their markers and the foundations on a chart change, which the new chart may not declare', () => {
    const next = reducer(ANSWERED, { type: 'setTarget', target: { chart: 'trust-anchor', chartVersion: '' } })
    expect(next.answers.values).toEqual({})
    expect(next.open).toEqual(['target.namespace'])
    expect(next.foundations).toBeUndefined()
  })

  it('keeps answers, markers and foundations when the chart stays', () => {
    const next = reducer(ANSWERED, { type: 'setTarget', target: { namespace: 'other' } })
    expect(next.answers.values).toBe(ANSWERED.answers.values)
    expect(next.open).toBe(ANSWERED.open)
    expect(next.foundations).toBe(ANSWERED.foundations)
  })

  it('drops the answers and markers the loaded foundations do not declare', () => {
    const next = reducer(ANSWERED, { type: 'foundationsLoaded', foundations: FOUNDATIONS.foundations })
    expect(next.answers.values).toEqual({ 'domain.base': 'example.org' })
    expect(next.open).toEqual(['target.namespace'])
  })

  it('keeps the answers object when the loaded foundations declare every answer, so the saved profile stays current', () => {
    const declared = { ...ANSWERED, answers: { ...ANSWERED.answers, values: { 'domain.base': 'example.org' } }, open: ['answers.domain.base'] as AppState['open'] }
    const next = reducer(declared, { type: 'foundationsLoaded', foundations: FOUNDATIONS.foundations })
    expect(next.answers).toBe(declared.answers)
    expect(next.open).toBe(declared.open)
  })

  it('takes the carried answers on a chart switch and drops the markers of the dropped ones', () => {
    const carried: Profile = { ...SAVED_PROFILE, target: { ...SAVED_PROFILE.target, chart: 'trust-anchor' }, role: 'operator', answers: { 'domain.base': 'example.org' } }
    const chartSwitch = {
      fromChart: 'data-space-connector',
      fromVersion: '10.16.0',
      toChart: 'trust-anchor',
      droppedChoices: [],
      carriedAnswers: ['domain.base'],
      droppedAnswers: ['host.verifier'],
    }
    const next = reducer(ANSWERED, { type: 'roleApplied', profile: carried, chartSwitch, change: null })
    expect(next.answers.values).toEqual({ 'domain.base': 'example.org' })
    expect(next.open).toEqual(['target.namespace'])
    expect(next.foundations).toBeUndefined()
  })

  it('keeps an answer still being typed on a preset that keeps the chart, which the request left out', () => {
    const typing = { ...ANSWERED, answers: { ...ANSWERED.answers, values: { 'domain.base': 'example' } } }
    const preset: Profile = { ...SAVED_PROFILE, target: { ...SAVED_PROFILE.target, chart: 'data-space-connector' }, role: 'provider', answers: {} }
    const next = reducer(typing, { type: 'roleApplied', profile: preset, chartSwitch: null, change: null })
    expect(next.answers.values).toBe(typing.answers.values)
    expect(next.open).toEqual(typing.open)
    expect(next.foundations).toBe(typing.foundations)
  })

  it.each<Action>([{ type: 'newConfiguration' }, { type: 'fileLoaded', report: REPORT, file: FILE }])('forgets the foundations on $type', (action) => {
    expect(reducer(ANSWERED, action).foundations).toBeUndefined()
  })
})
