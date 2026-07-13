// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Straw',
  tagline: 'A small, self-hosted HTTP/HTTPS egress proxy',
  favicon: 'img/logo.svg',

  future: {
    v4: true,
  },

  url: 'https://beremaran.github.io',
  baseUrl: '/straw-oss/',

  organizationName: 'beremaran',
  projectName: 'straw-oss',

  onBrokenLinks: 'throw',

  markdown: {
    mermaid: true,
  },

  themes: ['@docusaurus/theme-mermaid'],

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: '../docs/public',
          routeBasePath: 'docs',
          sidebarPath: './sidebars.js',
          editUrl: ({docPath}) => `https://github.com/beremaran/straw-oss/edit/main/docs/public/${docPath}`,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  plugins: [],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: 'Straw',
        logo: {
          alt: 'Straw logo',
          src: 'img/logo.svg',
        },
        items: [
          {label: 'Learn', to: '/docs/quickstart', position: 'left'},
          {label: 'Use', to: '/docs/api/requests', position: 'left'},
          {label: 'Operate', to: '/docs/deployment', position: 'left'},
          {label: 'Reference', to: '/docs/configuration', position: 'left'},
          {label: 'Contribute', to: '/docs/development', position: 'left'},
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            docsPluginId: 'default',
            position: 'left',
            label: 'All docs',
          },
          {
            href: 'https://github.com/beremaran/straw-oss',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Get started',
            items: [
              {label: 'Quickstart', to: '/docs/quickstart'},
              {label: 'Architecture', to: '/docs/architecture'},
              {label: 'Client SDKs', to: '/docs/sdk'},
              {label: 'CLI', to: '/docs/cli'},
              {label: 'Egress workers', to: '/docs/egress_worker'},
            ],
          },
          {
            title: 'Run Straw',
            items: [
              {label: 'Request API', to: '/docs/api/requests'},
              {label: 'Deployment', to: '/docs/deployment'},
              {label: 'Operations', to: '/docs/operations'},
              {label: 'Security', to: '/docs/security'},
              {label: 'Troubleshooting', to: '/docs/troubleshooting'},
            ],
          },
          {
            title: 'Project',
            items: [
              {label: 'GitHub', href: 'https://github.com/beremaran/straw-oss'},
              {label: 'Support policy', href: 'https://github.com/beremaran/straw-oss/blob/main/SUPPORT.md'},
              {label: 'Contributing', href: 'https://github.com/beremaran/straw-oss/blob/main/CONTRIBUTING.md'},
              {label: 'Security policy', href: 'https://github.com/beremaran/straw-oss/blob/main/SECURITY.md'},
              {label: 'Governance', href: 'https://github.com/beremaran/straw-oss/blob/main/GOVERNANCE.md'},
              {label: 'Changelog', href: 'https://github.com/beremaran/straw-oss/blob/main/CHANGELOG.md'},
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Berke Arslan. MIT licensed.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
      },
    }),
};

export default config;
