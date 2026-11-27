/** The window chrome every stage after S0 hangs in. */
import type { StageDef } from '../stages'
import { Expert } from '../stages/Expert'
import { Placeholder } from '../stages/Placeholder'
import { Skipped } from '../stages/Skipped'
import { useStore } from '../state/context'
import { isSkipped } from '../state/issues'
import { BundleDataError, useBundleData } from './bundleData'
import { Footer } from './Footer'
import { Rail } from './Rail'
import { SaveProvider } from './SaveDialog'
import { TitleBar } from './TitleBar'

/** Title bar, rail, content pane and footer around the current stage. */
export function Chrome({ stages }: { stages: readonly StageDef[] }) {
  const { state } = useStore()
  const current = stages.find((stage) => stage.id === state.stage)
  const Stage = current?.component
  const label = current?.label ?? state.stage
  const loadError = useBundleData()
  return (
    <BundleDataError.Provider value={loadError}>
      <SaveProvider>
        <div className="app">
          <TitleBar />
          <div className="body">
            <Rail stages={stages} />
            <main className="content">
              {state.expert ? (
                <Expert />
              ) : isSkipped(state, state.stage) ? (
                <Skipped label={label} />
              ) : Stage ? (
                <Stage />
              ) : (
                <Placeholder label={label} comingIn={current?.comingIn} stageId={state.stage} />
              )}
            </main>
          </div>
          <Footer stages={stages} />
        </div>
      </SaveProvider>
    </BundleDataError.Provider>
  )
}
