/** S7 Review: open questions, hand-set answers, deliberate feature choices and the load report of an opened values.yaml; the validation and generate panes come with Epic 6. */
import { useState } from 'react'
import { useSave } from '../chrome/saveContext'
import { loadValues, RootSource, VersionSource, type DerivedAnswer, type LoadReport } from '../api/load'
import { useStore } from '../state/context'
import { openQuestions } from '../state/openQuestions'
import { carriedOverCount, understoodCount, underivedAnswers } from '../state/report'
import { STAGE_LABELS, StageId } from '../state/stageIds'
import { DeliberateChoices } from './DeliberateChoices'
import { AnswerList } from './DerivedAnswers'
import { EntryTable, Overrides } from './Overrides'
import { HandSet } from './HandSet'
import { Placeholder } from './Placeholder'
import { RoleChanges } from './RoleChanges'
import { TrialReview } from './TrialReview'

/** What Review shows without a loaded file. */
export const REVIEW_COMING_IN = 'Epic 6 — Validation and review'

function isStageId(id: string): id is StageId {
  return id in STAGE_LABELS
}

function byStage(answers: readonly DerivedAnswer[]): [string, DerivedAnswer[]][] {
  const groups = new Map<string, DerivedAnswer[]>()
  for (const answer of answers) groups.set(answer.stage, [...(groups.get(answer.stage) ?? []), answer])
  return [...groups]
}

/** Label of the root picker. */
export const ROOT_LABEL = 'Read the chart’s values from'
/** How the picker names the document root. */
export const WHOLE_FILE = 'the whole file'

function rootName(root: string): string {
  return root === '' ? WHOLE_FILE : root
}

/** Where the chart's values were read from; another candidate re-reads the same file under that root.
 *  A re-read is never aborted: the server swaps its slot anyway, so the UI must follow it (see `rootChanged`).
 *  The in-flight flag lives in the store, so only one re-read runs and server and UI apply them in one order. */
function RootPicker({ report, file }: { report: LoadReport; file?: File }) {
  const { state, dispatch } = useStore()
  const busy = state.rereading
  const [error, setError] = useState<string>()
  const roots = [...new Set([report.root, ...report.rootCandidates, ''])]
  const how = report.rootFrom === RootSource.Detected ? 'detected' : 'as chosen'

  async function reread(root: string) {
    if (!file || busy) return
    dispatch({ type: 'rootRereadStarted' })
    setError(undefined)
    try {
      dispatch({ type: 'rootChanged', report: await loadValues(file, { root }), file })
    } catch (failure) {
      dispatch({ type: 'rootRereadFailed' })
      setError(failure instanceof Error ? failure.message : String(failure))
    }
  }

  return (
    <div className="panel">
      <h3>Chart values</h3>
      <p>
        Read from {report.root === '' ? WHOLE_FILE : <code>{report.root}</code>} ({how}). Keys outside it are carried over untouched.
      </p>
      {file && roots.length > 1 && (
        <label>
          {ROOT_LABEL}{' '}
          <select value={report.root} disabled={busy} aria-busy={busy} onChange={(event) => void reread(event.target.value)}>
            {roots.map((root) => (
              <option key={root} value={root}>
                {rootName(root)}
              </option>
            ))}
          </select>
        </label>
      )}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

/** Every open question with its stage and a link that focuses its field. */
function OpenQuestions() {
  const { state, dispatch } = useStore()
  const questions = openQuestions(state)
  return (
    <div className="panel">
      <h3>Open questions ({questions.length})</h3>
      {questions.length === 0 ? (
        <p className="hint">None: every question is answered.</p>
      ) : (
        <ul className="open-questions">
          {questions.map((q) => (
            <li key={q.field}>
              <span>{q.label}</span>
              <span className="stage">{STAGE_LABELS[q.stage]}</span>
              {q.marked && <span className="badge">marked</span>}
              <button type="button" className="link" aria-label={`Go to ${q.label}`} onClick={() => dispatch({ type: 'focusField', field: q.field })}>
                go ↗
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Notes of the last open or save: a source that changed or is missing, an older bundle revision. */
function ProfileNotes() {
  const { state } = useStore()
  const notes = state.profileNotes ?? []
  if (notes.length === 0) return null
  return (
    <div className="panel" role="status">
      <h3>Profile</h3>
      <ul className="notes">
        {notes.map((note) => (
          <li key={note}>{note}</li>
        ))}
      </ul>
    </div>
  )
}

/** The actions under Review; export and install come with Epic 6/7. */
function Actions() {
  const { save, busy, disabledReason } = useSave()
  return (
    <div className="actions">
      <button type="button" className="sec" disabled={busy || disabledReason !== undefined} title={disabledReason} onClick={save}>
        Save profile
      </button>
    </div>
  )
}

function Counts({ report }: { report: LoadReport }) {
  const rows: [string, number][] = [
    ['Owned by dsc-config', report.counts.owned],
    ['Recognized', report.counts.recognized],
    ['Unknown, kept verbatim', report.counts.unknown],
    ['Skipped pairs, kept verbatim', report.counts.skipped],
  ]
  return (
    <dl className="report-counts" aria-label="Counts">
      {rows.map(([label, count]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{count}</dd>
        </div>
      ))}
    </dl>
  )
}

function Answers({ report }: { report: LoadReport }) {
  const { dispatch } = useStore()
  const underived = underivedAnswers(report)
  return (
    <div className="panel">
      <h3>Answers read from the file</h3>
      {report.answers.length === 0 && <p className="hint">None: no key of the file matches a question of chart {report.chartVersion}.</p>}
      {byStage(report.answers).map(([stage, answers]) => (
        <div key={stage} className="stage-answers">
          <h4>
            {isStageId(stage) ? STAGE_LABELS[stage] : stage}
            {isStageId(stage) && (
              <button type="button" className="link" onClick={() => dispatch({ type: 'goTo', stage })}>
                go ↗
              </button>
            )}
          </h4>
          <AnswerList answers={answers} />
        </div>
      ))}
      {underived.length > 0 && (
        <p className="hint">Named by an owned marker but not in the file, so the profile has to supply them: {underived.join(', ')}</p>
      )}
    </div>
  )
}

function Report({ report, file }: { report: LoadReport; file?: File }) {
  const version =
    report.chartVersionFrom === VersionSource.Header ? 'from the file’s dsc-config header' : `latest bundle, because ${report.chartVersionNote ?? 'the header names none'}`
  return (
    <section>
      <h1>
        Opened <code>{report.source}</code>: understood {understoodCount(report)}, carried over untouched {carriedOverCount(report)}
      </h1>
      <p className="lede">
        Chart {report.chart} {report.chartVersion} ({version}).
      </p>
      <ProfileNotes />
      <OpenQuestions />
      <HandSet />
      <DeliberateChoices />
      <RoleChanges />
      <RootPicker report={report} file={file} />
      <div className="panel">
        <h3>Classification of {report.counts.total} keys</h3>
        <Counts report={report} />
      </div>
      <Answers report={report} />
      <div className="panel">
        <h3>Understood</h3>
        <details>
          <summary>Owned by dsc-config ({report.owned.length})</summary>
          <EntryTable entries={report.owned} />
        </details>
        <details>
          <summary>Recognized ({report.recognized.length})</summary>
          <EntryTable entries={report.recognized} />
        </details>
      </div>
      <div className="panel">
        <h3>Carried over untouched</h3>
        <Overrides report={report} />
      </div>
      <Actions />
    </section>
  )
}

/** Review stage: the trial's releases in the local trial; else profile notes, open questions and hand-set answers above the load report
 *  when a file was opened, otherwise above the Epic 6 placeholder. */
export function Review() {
  const { state } = useStore()
  if (state.trial) {
    return (
      <TrialReview trial={state.trial}>
        <Actions />
      </TrialReview>
    )
  }
  return state.loaded ? (
    <Report report={state.loaded} file={state.loadedFile} />
  ) : (
    <Placeholder label={STAGE_LABELS[StageId.Review]} comingIn={REVIEW_COMING_IN}>
      <ProfileNotes />
      <OpenQuestions />
      <HandSet />
      <DeliberateChoices />
      <RoleChanges />
      <Actions />
    </Placeholder>
  )
}
