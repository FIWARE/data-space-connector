/** The store's state and its pure transition function. */
import { emptyAnswers, type Answers, type Session, type Target } from './answers'
import { FIRST_STAGE, type StageId } from './stageIds'

/** Store contents; `entered` is false while S0 (Start) is shown. */
export interface AppState {
  entered: boolean
  stage: StageId
  visited: readonly StageId[]
  answers: Answers
  session: Session
}

/** Every change the UI can make to the store. */
export type Action =
  | { type: 'newConfiguration' }
  | { type: 'goTo'; stage: StageId }
  | { type: 'setTarget'; target: Partial<Target> }
  | { type: 'setSession'; session: Partial<Session> }

/** The state before anything was chosen on S0. */
export const initialState: AppState = {
  entered: false,
  stage: FIRST_STAGE,
  visited: [],
  answers: emptyAnswers(),
  session: {},
}

function visit(visited: readonly StageId[], stage: StageId): readonly StageId[] {
  return visited.includes(stage) ? visited : [...visited, stage]
}

/** Pure transition function of the store. */
export function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'newConfiguration':
      // The kubeconfig path is kept for the new configuration; the old one's context pick is not.
      return { ...state, entered: true, stage: FIRST_STAGE, visited: [FIRST_STAGE], answers: emptyAnswers(), session: { ...state.session, chosenContext: undefined } }
    case 'goTo':
      return { ...state, stage: action.stage, visited: visit(state.visited, action.stage) }
    case 'setTarget':
      return { ...state, answers: { ...state.answers, target: { ...state.answers.target, ...action.target } } }
    case 'setSession':
      return { ...state, session: { ...state.session, ...action.session } }
  }
}

