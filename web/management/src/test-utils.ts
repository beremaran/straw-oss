import type { AppState } from './types.js'

export function testState(overrides: Partial<AppState> = {}): AppState {
  return {
    baseUrl: 'http://localhost:8081',
    token: 'test-token',
    remember: false,
    currentPage: '#/overview',
    isLoading: false,
    error: null,
    toast: null,
    confirmDialog: null,
    overviewData: null,
    apiKeysData: null,
    rulesData: null,
    endpointsData: null,
    fingerprintsData: null,
    usageData: null,
    cacheData: null,
    systemData: null,
    ...overrides
  }
}

export function mustQuery<T extends Element>(root: ParentNode, selector: string): T {
  const el = root.querySelector<T>(selector)
  if (!el) throw new Error(`Missing test element: ${selector}`)
  return el
}

export function firstCall<TArgs extends unknown[]>(mock: { mock: { calls: TArgs[] } }): TArgs {
  const call = mock.mock.calls[0]
  if (!call) throw new Error('Expected mock to be called')
  return call
}
