/** The stage rail: jumpable at any time, with a status glyph per stage. */
import type { StageDef } from '../stages'
import { STATUS_GLYPH, STATUS_LEGEND, stageStatus } from '../state/issues'
import { useStore } from '../state/context'
import { useSave } from './saveContext'

/** Left rail of the chrome. */
export function Rail({ stages }: { stages: readonly StageDef[] }) {
  const { state, dispatch } = useStore()
  const { save, busy, saved, disabledReason } = useSave()
  return (
    <nav className="rail" aria-label="Stages">
      <ol>
        {stages.map((stage, index) => {
          const status = stageStatus(state, stage)
          return (
            <li key={stage.id}>
              <button
                type="button"
                aria-current={stage.id === state.stage && !state.expert ? 'step' : undefined}
                onClick={() => dispatch({ type: 'goTo', stage: stage.id })}
              >
                <span className={`st ${status}`} title={STATUS_LEGEND[status]} aria-label={STATUS_LEGEND[status]}>
                  {STATUS_GLYPH[status]}
                </span>
                <span>
                  {index + 1} {stage.label}
                </span>
              </button>
            </li>
          )
        })}
      </ol>
      <hr />
      <button
        type="button"
        disabled={!state.loaded}
        aria-current={state.expert ? 'page' : undefined}
        title={state.loaded ? 'Overrides carried over from the opened file' : 'Not available yet (Epic 5)'}
        onClick={() => dispatch({ type: 'openExpert' })}
      >
        <kbd>⌘E</kbd> Expert: YAML
      </button>
      <button
        type="button"
        disabled={busy || disabledReason !== undefined}
        aria-busy={busy}
        title={disabledReason ?? 'Save the answers as a .dscprofile.yaml'}
        onClick={save}
      >
        <kbd>⌘S</kbd> Save profile
      </button>
      {saved && (
        <p className="hint" role="status">
          Saved {saved}
        </p>
      )}
    </nav>
  )
}
