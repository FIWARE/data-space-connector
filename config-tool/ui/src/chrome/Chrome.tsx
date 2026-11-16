/** The window chrome every stage after S0 hangs in. */
import type { StageDef } from '../stages'
import { Placeholder } from '../stages/Placeholder'
import { useStore } from '../state/context'
import { Footer } from './Footer'
import { Rail } from './Rail'
import { TitleBar } from './TitleBar'

/** Title bar, rail, content pane and footer around the current stage. */
export function Chrome({ stages }: { stages: readonly StageDef[] }) {
  const { state } = useStore()
  const current = stages.find((stage) => stage.id === state.stage)
  const Stage = current?.component
  return (
    <div className="app">
      <TitleBar />
      <div className="body">
        <Rail stages={stages} />
        <main className="content">
          {Stage ? <Stage /> : <Placeholder label={current?.label ?? state.stage} comingIn={current?.comingIn} />}
        </main>
      </div>
      <Footer stages={stages} />
    </div>
  )
}
