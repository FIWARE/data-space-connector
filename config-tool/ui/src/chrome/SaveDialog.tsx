/** Saving the profile: in place once its path is known, otherwise through a dialog that asks for the path. */
import { useEffect, useState, type FormEvent, type KeyboardEvent, type ReactNode } from 'react'
import { ApiError } from '../api/client'
import { defaultProfilePath, fetchRecent, HTTP_CONFLICT, PATH_HINT, profileName, saveProfile } from '../api/profile'
import { useStore } from '../state/context'
import type { AppState } from '../state/reducer'
import { SaveContext } from './saveContext'

/** Accessible name of the dialog's path field. */
export const SAVE_PATH_LABEL = 'Profile path'
const SHORTCUT_KEY = 's'
/** Why the local trial cannot be saved as a profile. */
export const TRIAL_SAVE_DISABLED = 'The local trial is fixed and cannot be saved as a profile'

/** What the open dialog shows. */
interface DialogState {
  path: string
  error?: string
  /** The path exists; submitting again replaces it. */
  confirmReplace?: boolean
  /** Notes of a save that went through; the dialog stays open to show them. */
  notes?: readonly string[]
}

function message(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}

interface DialogProps {
  dialog: DialogState
  busy: boolean
  source?: string
  onPath: (path: string) => void
  onSubmit: () => void
  onClose: () => void
}

/** The path form, or the notes of the save it just made. */
function SaveDialog({ dialog, busy, source, onPath, onSubmit, onClose }: DialogProps) {
  function submit(event: FormEvent) {
    event.preventDefault()
    if (!busy) onSubmit()
  }
  function keyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') onClose()
  }
  return (
    <div className="overlay">
      <form role="dialog" aria-modal="true" aria-labelledby="save-title" className="dialog panel" onSubmit={submit} onKeyDown={keyDown}>
        <h3 id="save-title">Save profile</h3>
        {dialog.notes ? (
          <>
            <p>
              Saved to <code>{dialog.path}</code>.
            </p>
            <ul className="notes" role="status">
              {dialog.notes.map((note) => (
                <li key={note}>{note}</li>
              ))}
            </ul>
            <div className="actions">
              <button type="button" className="primary" onClick={onClose}>
                Done
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="row">
              <label htmlFor="save-path">{SAVE_PATH_LABEL}</label>
              <input id="save-path" type="text" value={dialog.path} autoFocus spellCheck={false} onChange={(event) => onPath(event.target.value)} />
            </div>
            <p className="hint">{PATH_HINT}</p>
            {source && (
              <p className="hint">
                Save next to <code>{source}</code> so reopening finds its overrides.
              </p>
            )}
            {dialog.confirmReplace && (
              <p className="field-error" role="alert">
                {dialog.path} exists. Replace it?
              </p>
            )}
            {dialog.error && (
              <p className="field-error" role="alert">
                {dialog.error}
              </p>
            )}
            <div className="actions">
              <button type="button" className="sec" onClick={onClose}>
                Cancel
              </button>
              <button type="submit" className="primary" disabled={busy} aria-busy={busy}>
                {dialog.confirmReplace ? 'Replace' : 'Save'}
              </button>
            </div>
          </>
        )}
      </form>
    </div>
  )
}

/** The profile last written and the state it was written from; an edit since then makes it stale. */
interface Written {
  name: string
  answers: AppState['answers']
  open: AppState['open']
  loaded: AppState['loaded']
}

/** Provides the save action to the chrome, binds ⌘S / Ctrl+S and shows the dialog.
 *  Format errors do not block a save: a half-finished configuration is saved as typed (ADR 0015 §5). */
export function SaveProvider({ children }: { children: ReactNode }) {
  const { state, dispatch } = useStore()
  const [dialog, setDialog] = useState<DialogState>()
  const [busy, setBusy] = useState(false)
  const [written, setWritten] = useState<Written>()
  const saved = written && written.answers === state.answers && written.open === state.open && written.loaded === state.loaded ? written.name : undefined

  // Returns what the dialog shows next; undefined closes it. Notes of an in-place save go to Review only.
  async function write(path: string, overwrite: boolean, inDialog: boolean): Promise<DialogState | undefined> {
    try {
      const response = await saveProfile(state, path, overwrite)
      dispatch({ type: 'profileSaved', path: response.path, profile: response.profile, notes: response.notes })
      setWritten({ name: profileName(response.path), answers: state.answers, open: state.open, loaded: state.loaded })
      return inDialog && response.notes.length > 0 ? { path: response.path, notes: response.notes } : undefined
    } catch (failure) {
      if (failure instanceof ApiError && failure.status === HTTP_CONFLICT && !overwrite) return { path, confirmReplace: true }
      return { path, error: message(failure) }
    }
  }

  async function run(path: string, overwrite: boolean, inDialog: boolean) {
    setBusy(true)
    setWritten(undefined)
    setDialog(await write(path, overwrite, inDialog))
    setBusy(false)
  }

  const disabledReason = state.trial ? TRIAL_SAVE_DISABLED : undefined

  async function save() {
    if (busy || dialog || disabledReason) return
    // A known path is this configuration's own profile, so it is replaced without asking.
    if (state.profilePath) return run(state.profilePath, true, false)
    setBusy(true)
    let dir = ''
    try {
      dir = (await fetchRecent()).defaultDir
    } catch {
      // Without a default directory the field holds only the file name; the user types the rest.
    }
    setBusy(false)
    setDialog({ path: defaultProfilePath(dir, state.answers.target.releaseName) })
  }

  useEffect(() => {
    function onKey(event: globalThis.KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === SHORTCUT_KEY) {
        event.preventDefault()
        void save()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  return (
    <SaveContext.Provider value={{ save: () => void save(), busy, saved, disabledReason }}>
      {children}
      {dialog && (
        <SaveDialog
          dialog={dialog}
          busy={busy}
          source={state.loaded?.source}
          onPath={(path) => setDialog({ path })}
          onSubmit={() => void run(dialog.path, dialog.confirmReplace ?? false, true)}
          onClose={() => setDialog(undefined)}
        />
      )}
    </SaveContext.Provider>
  )
}
