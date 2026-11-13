/** The title bar's context line, derived from the store. */
import type { AppState } from './reducer'

/** The app's name; Go's launcher.WindowTitle and index.html carry the same string. */
export const APP_TITLE = 'DSC Configurator'

const SEPARATOR = ' · '

/** `<namespace> · <role> · chart <version>`, leaving out whatever is not answered yet. */
export function titleContext(state: AppState): string {
  const { namespace, chartVersion } = state.answers.target
  return [namespace, chartVersion && `chart ${chartVersion}`].filter(Boolean).join(SEPARATOR)
}
