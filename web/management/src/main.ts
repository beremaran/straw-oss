import { subscribe } from './state.js'
import { initRouter, handleRouteChange } from './router.js'

// Initialize Router
initRouter()

// Subscribe to state changes to update the shell layout, toasts, and dialogs.
// We only trigger re-route/re-render when state elements change.
subscribe(() => {
  handleRouteChange()
})
