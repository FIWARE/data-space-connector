/** The store's state and its pure transition function. */
import { VersionSource, type LoadReport } from '../api/load'
import { fromProfile, type ModelRef, type Profile } from '../api/profile'
import type { Trial } from '../api/trial'
import { emptyAnswers, Environment, type Answers, type Session, type Target } from './answers'
import { fieldStage, type FieldId } from './openQuestions'
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
  focus?: FieldId
  /** Where this configuration was last saved or opened from; ⌘S saves there in place. */
  profilePath?: string
  /** The bundle the profile was built with; the server keeps it while no chart version is answered. */
  profileModel?: ModelRef
  /** Notes of the last open or save (source changed or missing, older revision); Review shows them. */
  profileNotes?: readonly string[]
  /** The bundled local trial; set only by `trialStarted`, any other start over drops it. */
  trial?: Trial
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
  | { type: 'focusField'; field: FieldId }
  | { type: 'focusDone' }
  | { type: 'profileOpened'; path: string; profile: Profile; report?: LoadReport; notes: readonly string[] }
  | { type: 'profileSaved'; path: string; profile: Profile; notes: readonly string[] }
  | { type: 'trialStarted'; trial: Trial }

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
}

function visit(visited: readonly StageId[], stage: StageId): readonly StageId[] {
  return visited.includes(stage) ? visited : [...visited, stage]
}

function goTo(state: AppState, stage: StageId): AppState {
  return { ...state, stage, visited: visit(state.visited, stage), expert: false }
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
    profileNotes: undefined,
    trial: undefined,
  }
}

// The trial is a fixed development configuration that lands on Review; stages 1–6 are skipped, so none is visited.
function trialStarted(state: AppState, trial: Trial): AppState {
  const answers = emptyAnswers()
  answers.target.environment = Environment.Development
  return { ...startOver(state, StageId.Review, answers), trial }
}

// The recorded root is used for an opened profile's source, so there is no file to re-read under another one.
function profileOpened(state: AppState, action: Extract<Action, { type: 'profileOpened' }>): AppState {
  const { answers, open } = fromProfile(action.profile)
  return {
    ...startOver(state, StageId.Review, answers, action.report),
    open,
    profilePath: action.path,
    profileModel: action.profile.model,
    profileNotes: action.notes,
  }
}

function loadedAnswers(report: LoadReport): Answers {
  const answers = emptyAnswers()
  if (report.chartVersionFrom === VersionSource.Header) {
    answers.target.chartVersion = report.chartVersion
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
      return { ...state, profilePath: action.path, profileModel: action.profile.model, profileNotes: action.notes }
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
      return { ...state, answers: { ...state.answers, target: { ...state.answers.target, ...action.target } } }
    case 'setSession':
      return { ...state, session: { ...state.session, ...action.session } }
  }
}

