/** The store's state and its pure transition function. */
import { VersionSource, type LoadReport } from '../api/load'
import { answerValues, fromProfile, type ModelRef, type Profile } from '../api/profile'
import type { Foundation } from '../api/foundations'
import type { ChartSwitch, RoleChange } from '../api/roles'
import type { Trial } from '../api/trial'
import { dropMismatched } from './foundations'
import { emptyAnswers, Environment, type Answers, type AnswerValue, type FeatureState, type Session, type Target } from './answers'
import { answerFieldId, answerId, fieldStage, isAnswerFieldId, type FieldId, type QuestionId } from './openQuestions'
import { FIRST_STAGE, StageId } from './stageIds'

/** Store contents; `entered` is false while S0 (Start) is shown. */
export interface AppState {
  entered: boolean
  stage: StageId
  visited: readonly StageId[]
  answers: Answers
  session: Session
  /** Report of the opened values.yaml; kept outside `answers` until later epics own its answers. */
  loaded?: LoadReport
  /** The opened file itself, so it can be re-read with another root; never decoded. */
  loadedFile?: File
  /** The Expert view covers the current stage; any stage jump closes it. */
  expert: boolean
  /** A root re-read is in flight; kept here so the picker stays disabled when Review remounts. */
  rereading: boolean
  /** Fields marked "ask later", sorted by id as the profile's `open`. */
  open: readonly FieldId[]
  /** A field its stage should focus once shown; the stage clears it with `focusDone`. */
  focus?: QuestionId
  /** Where this configuration was last saved or opened from; ⌘S saves there in place. */
  profilePath?: string
  /** The bundle the profile was built with; the server keeps it while no chart version is answered. */
  profileModel?: ModelRef
  /** The bundle the role catalogue came from when that is not `profileModel`; the server recomputes it on save. */
  profileCatalogue?: ModelRef
  /** Notes of the last open or save (source changed or missing, older revision); Review shows them. */
  profileNotes?: readonly string[]
  /** The bundled local trial; set only by `trialStarted`, any other start over drops it. */
  trial?: Trial
  /** A role switched the chart and cleared its version; Stage 1 asks for it instead of preselecting latest. */
  versionOpen: boolean
  /** One-shot notice of the last role pick that switched the chart; any stage jump drops it. */
  chartSwitch?: ChartSwitch
  /** One-shot notice of a role cleared by a chart change on Stage 1; any stage jump drops it. */
  roleCleared?: RoleCleared
  /** Role changes applied since this configuration was started or opened; session history Review lists (ADR 0019). */
  roleChanges: readonly AppliedRoleChange[]
  /** The foundations of the target chart once fetched; Review names marked answers by their labels. */
  foundations?: readonly Foundation[]
}

/** A confirmed role change and the chart switch it made, if any. */
export interface AppliedRoleChange {
  change: RoleChange
  chartSwitch: ChartSwitch | null
}

/** A role that no longer fits the target chart, cleared by a chart change. */
export interface RoleCleared {
  role: string
  fromChart: string
  toChart: string
}

/** Every change the UI can make to the store. */
export type Action =
  | { type: 'newConfiguration' }
  | { type: 'goTo'; stage: StageId }
  | { type: 'setTarget'; target: Partial<Target> }
  | { type: 'setSession'; session: Partial<Session> }
  | { type: 'fileLoaded'; report: LoadReport; file: File }
  | { type: 'rootRereadStarted' }
  | { type: 'rootChanged'; report: LoadReport; file: File }
  | { type: 'rootRereadFailed' }
  | { type: 'openExpert' }
  | { type: 'toggleOpen'; field: FieldId }
  | { type: 'focusField'; field: QuestionId }
  | { type: 'focusDone' }
  | { type: 'profileOpened'; path: string; profile: Profile; report?: LoadReport; notes: readonly string[] }
  | { type: 'profileSaved'; path: string; profile: Profile; notes: readonly string[] }
  | { type: 'trialStarted'; trial: Trial }
  | { type: 'roleApplied'; profile: Profile; chartSwitch: ChartSwitch | null; change: RoleChange | null }
  | { type: 'featuresChanged'; features: Record<string, FeatureState> }
  | { type: 'setAnswer'; id: string; value: string }
  | { type: 'foundationsLoaded'; foundations: readonly Foundation[] }

/** The state before anything was chosen on S0. */
export const initialState: AppState = {
  entered: false,
  stage: FIRST_STAGE,
  visited: [],
  answers: emptyAnswers(),
  session: {},
  expert: false,
  rereading: false,
  open: [],
  versionOpen: false,
  roleChanges: [],
}

function visit(visited: readonly StageId[], stage: StageId): readonly StageId[] {
  return visited.includes(stage) ? visited : [...visited, stage]
}

function goTo(state: AppState, stage: StageId): AppState {
  return { ...state, stage, visited: visit(state.visited, stage), expert: false, chartSwitch: undefined, roleCleared: undefined }
}

function toggle(open: readonly FieldId[], field: FieldId): readonly FieldId[] {
  return open.includes(field) ? open.filter((f) => f !== field) : [...open, field].sort()
}

// A new or opened configuration starts over; the kubeconfig path is kept, the old context pick is not.
// The profile is dropped too, so ⌘S never overwrites the previous profile with an unrelated configuration.
function startOver(state: AppState, stage: StageId, answers: Answers, loaded?: LoadReport, loadedFile?: File): AppState {
  return {
    ...state,
    entered: true,
    stage,
    visited: [stage],
    answers,
    session: { ...state.session, chosenContext: undefined },
    loaded,
    loadedFile,
    expert: false,
    open: [],
    focus: undefined,
    profilePath: undefined,
    profileModel: undefined,
    profileCatalogue: undefined,
    profileNotes: undefined,
    trial: undefined,
    versionOpen: false,
    chartSwitch: undefined,
    roleCleared: undefined,
    roleChanges: [],
    foundations: undefined,
  }
}

// The trial is a fixed development configuration that lands on Review; stages 1–6 are skipped, so none is visited.
function trialStarted(state: AppState, trial: Trial): AppState {
  const answers = emptyAnswers()
  answers.target.environment = Environment.Development
  return { ...startOver(state, StageId.Review, answers), trial }
}

// The recorded root is used for an opened profile's source, so there is no file to re-read under another one.
// A saved role without a chart version came from a chart switch, so its version is asked, not guessed.
function profileOpened(state: AppState, action: Extract<Action, { type: 'profileOpened' }>): AppState {
  const { answers, open } = fromProfile(action.profile)
  return {
    ...startOver(state, StageId.Review, answers, action.report),
    open,
    profilePath: action.path,
    profileModel: action.profile.model,
    profileCatalogue: action.profile.catalogue,
    profileNotes: action.notes,
    versionOpen: answers.role !== undefined && answers.target.chartVersion === '',
  }
}

// The first pick has no change to record. A chart switch takes the answers the server carried and drops the rest; a
// preset that keeps the chart keeps the client's, since the request left out the ones still being typed.
function roleApplied(state: AppState, { profile, chartSwitch, change }: Extract<Action, { type: 'roleApplied' }>): AppState {
  const target = { ...state.answers.target, chart: profile.target.chart, chartVersion: profile.target.chartVersion }
  const dropped = new Set<FieldId>((chartSwitch?.droppedAnswers ?? []).map(answerFieldId))
  const values = chartSwitch ? answerValues(profile.answers) : state.answers.values
  return {
    ...state,
    answers: { ...state.answers, target, role: profile.role, features: profile.features ?? {}, values },
    open: state.open.filter((field) => !dropped.has(field)),
    foundations: chartSwitch ? undefined : state.foundations,
    profileModel: profile.model,
    profileCatalogue: profile.catalogue,
    versionOpen: chartSwitch !== null || state.versionOpen,
    chartSwitch: chartSwitch ?? undefined,
    roleCleared: undefined,
    roleChanges: change ? [...state.roleChanges, { change, chartSwitch }] : state.roleChanges,
  }
}

// Every role deploys exactly one chart, so a chart change drops the role rather than leave one the server refuses.
// The answers go too: which ids the new chart declares is only known once its foundations arrive, and an undeclared
// one would make every profile request a 422. A role pick that switches charts carries them (ADR 0019).
function setTarget(state: AppState, change: Partial<Target>): AppState {
  const before = state.answers.target
  const target = { ...before, ...change }
  const chartChanged = target.chart !== before.chart
  const versionOpen = state.versionOpen && !chartChanged && !change.chartVersion
  if (!chartChanged) return { ...state, answers: { ...state.answers, target }, versionOpen }
  const role = state.answers.role
  return {
    ...state,
    answers: { ...state.answers, target, role: undefined, features: {}, values: {} },
    open: state.open.filter((field) => !isAnswerFieldId(field)),
    foundations: undefined,
    profileCatalogue: undefined,
    versionOpen,
    chartSwitch: undefined,
    roleCleared: role === undefined ? undefined : { role, fromChart: before.chart, toChart: target.chart },
  }
}

// An empty value is no answer, so clearing a field leaves the chart default in place.
function setAnswer(values: Record<string, AnswerValue>, id: string, value: string): Record<string, AnswerValue> {
  if (value !== '') return { ...values, [id]: value }
  return Object.fromEntries(Object.entries(values).filter(([key]) => key !== id))
}

// A chart version that no longer declares an answer drops it and its marker; the server would refuse both.
function foundationsLoaded(state: AppState, foundations: readonly Foundation[]): AppState {
  const declared = new Set(foundations.map((f) => f.id))
  const values = Object.fromEntries(Object.entries(state.answers.values).filter(([id]) => declared.has(id)))
  const open = state.open.filter((field) => !isAnswerFieldId(field) || declared.has(answerId(field)))
  const pruned = Object.keys(values).length !== Object.keys(state.answers.values).length
  // Unchanged answers and markers keep their objects, so loading the foundations does not make the saved profile stale.
  return {
    ...state,
    foundations,
    answers: pruned ? { ...state.answers, values } : state.answers,
    open: open.length === state.open.length ? state.open : open,
  }
}

function loadedAnswers(report: LoadReport): Answers {
  const answers = emptyAnswers()
  if (report.chartVersionFrom === VersionSource.Header) {
    answers.target.chartVersion = report.chartVersion
  }
  // A foundation answer the file derives counts as given, as the server seeds an opened profile (ADR 0022).
  for (const a of report.answers) {
    if (a.stage === StageId.Foundations && typeof a.value !== 'number') {
      answers.values[a.id] = a.value
    }
  }
  return answers
}

/** Pure transition function of the store. */
export function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'newConfiguration':
      return startOver(state, FIRST_STAGE, emptyAnswers())
    case 'fileLoaded':
      return startOver(state, StageId.Review, loadedAnswers(action.report), action.report, action.file)
    case 'profileOpened':
      return profileOpened(state, action)
    case 'trialStarted':
      return trialStarted(state, action.trial)
    case 'profileSaved':
      return {
        ...state,
        profilePath: action.path,
        profileModel: action.profile.model,
        profileCatalogue: action.profile.catalogue,
        profileNotes: action.notes,
      }
    case 'roleApplied':
      return roleApplied(state, action)
    case 'featuresChanged':
      // A new answers object is what makes the saved profile stale, as with every other edit.
      return { ...state, answers: { ...state.answers, features: action.features } }
    case 'setAnswer': {
      const values = dropMismatched(setAnswer(state.answers.values, action.id, action.value), action.id, state.foundations)
      return { ...state, answers: { ...state.answers, values } }
    }
    case 'foundationsLoaded':
      return foundationsLoaded(state, action.foundations)
    case 'rootRereadStarted':
      return { ...state, rereading: true }
    case 'rootChanged':
      // The same file under another root: not a new configuration, so stage, visits, answers and session stay.
      // A late answer for a file that has since been replaced or dropped is stale.
      return { ...state, rereading: false, loaded: state.loadedFile === action.file ? action.report : state.loaded }
    case 'rootRereadFailed':
      return { ...state, rereading: false }
    case 'goTo':
      return goTo(state, action.stage)
    case 'focusField':
      return { ...goTo(state, fieldStage(action.field)), focus: action.field }
    case 'focusDone':
      return { ...state, focus: undefined }
    case 'toggleOpen':
      // The value stays either way: marking makes it provisional, unmarking keeps it.
      return { ...state, open: toggle(state.open, action.field) }
    case 'openExpert':
      return { ...state, expert: true }
    case 'setTarget':
      return setTarget(state, action.target)
    case 'setSession':
      return { ...state, session: { ...state.session, ...action.session } }
  }
}

