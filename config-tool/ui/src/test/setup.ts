import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'
import { mockApi } from './api'

// jsdom does not lay out, so it has no scrollIntoView; Target calls it when a Review link focuses a field.
Element.prototype.scrollIntoView = () => {}

// Every screen may call the API on mount; tests that care install their own answers.
beforeEach(() => mockApi())
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
