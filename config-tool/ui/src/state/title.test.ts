/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest'
import indexHtml from '../../index.html?raw'
import { APP_TITLE } from './title'

describe('APP_TITLE', () => {
  it('matches the <title> of index.html, which Go pins to launcher.WindowTitle', () => {
    const page = new DOMParser().parseFromString(indexHtml, 'text/html')
    expect(page.title).toBe(APP_TITLE)
  })
})
