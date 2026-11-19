/** The title bar's context line, derived from the store. */
import { profileName } from '../api/profile'
import type { AppState } from './reducer'

/** The app's name; Go's launcher.WindowTitle and index.html carry the same string, and tests pin all three. */
export const APP_TITLE = 'DSC Configurator'

const SEPARATOR = ' · '

/** `<profile> · <namespace> · <role> · chart <version>`, leaving out whatever is not saved or answered yet. */
export function titleContext(state: AppState): string {
  const { namespace, chartVersion } = state.answers.target
  const profile = state.profilePath && profileName(state.profilePath)
  return [profile, namespace, chartVersion && `chart ${chartVersion}`].filter(Boolean).join(SEPARATOR)
}
