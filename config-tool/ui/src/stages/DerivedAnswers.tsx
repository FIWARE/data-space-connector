/** Read-only list of the answers a loaded file gave one stage, until that stage's epic owns them. */
import type { DerivedAnswer } from '../api/load'
import { useStore } from '../state/context'
import { formatAnswerValue } from '../state/report'
import type { StageId } from '../state/stageIds'

/** One answer per line: id, value, and the paths it was read from. */
export function AnswerList({ answers }: { answers: readonly DerivedAnswer[] }) {
  return (
    <ul className="answers">
      {answers.map((answer) => (
        <li key={answer.id}>
          <code>{answer.id}</code> = <strong>{formatAnswerValue(answer.value)}</strong>{' '}
          <span className="hint">
            ({answer.from}, from {answer.paths.join(', ')})
          </span>
        </li>
      ))}
    </ul>
  )
}

/** The loaded file's answers for `stage`; renders nothing when no file is loaded or none apply. */
export function DerivedAnswers({ stage }: { stage: StageId }) {
  const { state } = useStore()
  const answers = state.loaded?.answers.filter((answer) => answer.stage === stage) ?? []
  if (answers.length === 0) return null
  return (
    <div className="panel">
      <h3>Read from {state.loaded?.source}</h3>
      <AnswerList answers={answers} />
      <p className="hint">Shown read-only until this stage can be edited.</p>
    </div>
  )
}
