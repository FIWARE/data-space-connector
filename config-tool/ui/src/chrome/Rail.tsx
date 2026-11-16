/** The stage rail: jumpable at any time, with a status glyph per stage. */
import type { StageDef } from '../stages'
import { STATUS_GLYPH, STATUS_LEGEND, stageStatus } from '../state/issues'
import { useStore } from '../state/context'

/** Left rail of the chrome. */
export function Rail({ stages }: { stages: readonly StageDef[] }) {
  const { state, dispatch } = useStore()
  return (
    <nav className="rail" aria-label="Stages">
      <ol>
        {stages.map((stage, index) => {
          const status = stageStatus(state, stage)
          return (
            <li key={stage.id}>
              <button
                type="button"
                aria-current={stage.id === state.stage ? 'step' : undefined}
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
      <button type="button" disabled title="Not available yet (Epic 5)">
        <kbd>⌘E</kbd> Expert: YAML
      </button>
      <button type="button" disabled title="Not available yet (CT-4)">
        <kbd>⌘S</kbd> Save profile
      </button>
    </nav>
  )
}
