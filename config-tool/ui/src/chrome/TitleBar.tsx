/** The window title bar: app name plus what is known about the configuration. */
import { useStore } from '../state/context'
import { titleContext } from '../state/title'

/** Title bar of the chrome. */
export function TitleBar() {
  const { state } = useStore()
  return (
    <header className="bar title">
      <span className="name">DSC Configurator</span>
      <span className="ctx">{titleContext(state)}</span>
    </header>
  )
}
