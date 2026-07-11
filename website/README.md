# Straw Documentation Website

Docusaurus site that publishes consumer-facing documentation directly from the repo's existing public Markdown — no content is duplicated here:

- **Documentation Portal** (`/docs`) — sourced from `../docs/public/**` (public-facing quickstart, SDK integration, authentication, configuration, request forwarding, and operations guides).

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
at `beremaran.github.io/straw-oss`; update `url`/`baseUrl`/`organizationName`/`projectName` there
if hosting elsewhere.
