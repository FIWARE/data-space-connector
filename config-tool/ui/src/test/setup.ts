import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'
import { mockApi } from './api'

// Every screen may call the API on mount; tests that care install their own answers.
beforeEach(() => mockApi())
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
