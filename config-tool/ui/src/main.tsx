import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './styles.css'

const ROOT_ID = 'root'

const root = document.getElementById(ROOT_ID)
if (!root) {
  throw new Error(`#${ROOT_ID} missing from index.html`)
}
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
