// Straw Management UI Hash Router

import { state, setState } from './state.js';
import { LoginPage } from './pages/login.js';
import { OverviewPage } from './pages/overview.js';
import { ApiKeysPage } from './pages/api-keys.js';
import { RoutingRulesPage } from './pages/routing-rules.js';
import { RoutingRuleEditorPage } from './pages/routing-rule-editor.js';
import { EndpointsPage } from './pages/endpoints.js';
import { FingerprintsPage } from './pages/fingerprints.js';
import { UsagePage } from './pages/usage.js';
import { CachePage } from './pages/cache.js';
import { SystemPage } from './pages/system.js';
import { renderShell } from './components/shell.js';

const routes = {
  '#/login': LoginPage,
  '#/overview': OverviewPage,
  '#/api-keys': ApiKeysPage,
  '#/routing-rules': RoutingRulesPage,
  '#/routing-rules/new': RoutingRuleEditorPage,
  '#/routing-rules/edit': RoutingRuleEditorPage, // handled with query params e.g. #/routing-rules/edit?id=...
  '#/endpoints': EndpointsPage,
  '#/fingerprints': FingerprintsPage,
  '#/usage': UsagePage,
  '#/cache': CachePage,
  '#/system': SystemPage
};

export function initRouter() {
  window.addEventListener('hashchange', handleRouteChange);
  // Initial routing
  handleRouteChange();
}

export function handleRouteChange() {
  const hash = window.location.hash || '#/overview';
  
  // Normalize the route key
  let routeKey = hash;
  if (hash.startsWith('#/routing-rules/edit')) {
    routeKey = '#/routing-rules/edit';
  } else if (hash.startsWith('#/routing-rules/') && hash !== '#/routing-rules/new' && !hash.includes('?')) {
    // Dynamic rule detail, route to edit page or detail page
    routeKey = '#/routing-rules/edit';
  }

  // Auth guard
  const hasToken = !!state.token;
  if (!hasToken && routeKey !== '#/login') {
    window.location.hash = '#/login';
    return;
  }
  if (hasToken && routeKey === '#/login') {
    window.location.hash = '#/overview';
    return;
  }

  const page = routes[routeKey] || OverviewPage;
  setState({ currentPage: hash });

  const appDiv = document.querySelector('#app');
  if (routeKey === '#/login') {
    appDiv.innerHTML = page.render(state);
    if (page.afterRender) page.afterRender(state);
  } else {
    // Render inside the App Shell
    appDiv.innerHTML = renderShell(state, page.render(state));
    // Attach Shell events
    attachShellEvents();
    if (page.afterRender) page.afterRender(state);
  }
}

function attachShellEvents() {
  const signOutBtn = document.getElementById('shell-sign-out');
  if (signOutBtn) {
    signOutBtn.addEventListener('click', (e) => {
      e.preventDefault();
      import('./state.js').then(({ clearSession }) => {
        clearSession();
        window.location.hash = '#/login';
      });
    });
  }
  
  // Refresh button
  const refreshBtn = document.getElementById('shell-refresh');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', (e) => {
      e.preventDefault();
      const hash = window.location.hash || '#/overview';
      let routeKey = hash;
      if (hash.startsWith('#/routing-rules/edit') || (hash.startsWith('#/routing-rules/') && hash !== '#/routing-rules/new' && !hash.includes('?'))) {
        routeKey = '#/routing-rules/edit';
      }
      const page = routes[routeKey];
      if (page && page.refresh) {
        page.refresh(state);
      }
    });
  }
}
