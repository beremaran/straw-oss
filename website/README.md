# Straw Documentation Website

Docusaurus site that publishes two sections directly from the repo's existing Markdown — no
content is duplicated here:

- **SDK & API Docs** (`/docs`) — sourced from `../docs/public/**` (public-facing quickstart,
  auth, config, requests, admin API reference).
- **Internal Docs** (`/internal`) — sourced from `../docs/planning/**` (architecture and
  planning docs for internal developers).

Edit the Markdown in those source directories; this site just renders it.

## Local development

```bash
npm install
npm start
```

## Build

```bash
npm run build
```

Generates static output into `build/`, deployable to any static host (GitHub Pages,
Cloudflare Pages, Netlify, etc). `docusaurus.config.js` is currently set up for GitHub Pages
at `beremaran.github.io/straw`; update `url`/`baseUrl`/`organizationName`/`projectName` there
if hosting elsewhere.
