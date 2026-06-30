// App Shell Component Placeholder
export function renderShell(state, contentHtml) {
  return `
    <div class="app-shell">
      <header class="shell-header">
        <div class="logo">Straw Console</div>
        <button id="shell-refresh">Refresh</button>
        <button id="shell-sign-out">Sign Out</button>
      </header>
      <div class="shell-body">
        <aside class="shell-sidebar">
          <nav>
            <a href="#/overview">Overview</a>
            <a href="#/api-keys">API Keys</a>
            <a href="#/routing-rules">Routing Rules</a>
            <a href="#/endpoints">Endpoints</a>
            <a href="#/fingerprints">Fingerprints</a>
            <a href="#/usage">Usage</a>
            <a href="#/cache">Cache</a>
            <a href="#/system">System</a>
          </nav>
        </aside>
        <main class="shell-content">
          ${contentHtml}
        </main>
      </div>
    </div>
  `;
}
