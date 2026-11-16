import { describe, expect, it } from 'vitest'
import { initialState, reducer } from './reducer'

describe('reducer', () => {
  it('keeps the kubeconfig path but drops the context pick on a new configuration', () => {
    const state = { ...initialState, session: { kubeconfig: '/home/me/.kube/config', chosenContext: 'prod' } }
    expect(reducer(state, { type: 'newConfiguration' }).session).toEqual({ kubeconfig: '/home/me/.kube/config', chosenContext: undefined })
  })
})
