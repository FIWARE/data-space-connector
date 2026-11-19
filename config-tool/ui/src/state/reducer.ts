/** The store's state and its pure transition function. */
import { VersionSource, type LoadReport } from '../api/load'
import { emptyAnswers, type Answers, type Session, type Target } from './answers'
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
}

/** Every change the UI can make to the store. */
export type Action =
  | { type: 'newConfiguration' }
  | { type: 'goTo'; stage: StageId }
  | { type: 'setTarget'; target: Partial<Target> }
  | { type: 'setSession'; session: Partial<Session> }
  | { type: 'fileLoaded'; report: LoadReport; file: File }
  | { type: 'rootChanged'; report: LoadReport; file: File }
  | { type: 'openExpert' }

/** The state before anything was chosen on S0. */
export const initialState: AppState = {
  entered: false,
  stage: FIRST_STAGE,
  visited: [],
  answers: emptyAnswers(),
  session: {},
  expert: false,
}

function visit(visited: readonly StageId[], stage: StageId): readonly StageId[] {
  return visited.includes(stage) ? visited : [...visited, stage]
}

// A new or opened configuration starts over; the kubeconfig path is kept, the old context pick is not.
function startOver(state: AppState, stage: StageId, answers: Answers, loaded?: LoadReport, loadedFile?: File): AppState {
  return { ...state, entered: true, stage, visited: [stage], answers, session: { ...state.session, chosenContext: undefined }, loaded, loadedFile, expert: false }
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
    case 'rootChanged':
      // The same file under another root: not a new configuration, so stage, visits, answers and session stay.
      // A late answer for a file that has since been replaced or dropped is stale.
      return state.loadedFile === action.file ? { ...state, loaded: action.report } : state
    case 'goTo':
      return { ...state, stage: action.stage, visited: visit(state.visited, action.stage), expert: false }
    case 'openExpert':
      return { ...state, expert: true }
    case 'setTarget':
      return { ...state, answers: { ...state.answers, target: { ...state.answers.target, ...action.target } } }
    case 'setSession':
      return { ...state, session: { ...state.session, ...action.session } }
  }
}

