// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Straw',
  tagline: 'A small, self-hosted HTTP/HTTPS egress proxy',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://beremaran.github.io',
  baseUrl: '/straw-oss/',

  organizationName: 'beremaran',
  projectName: 'straw-oss',

  onBrokenLinks: 'throw',

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
          editUrl: 'https://github.com/beremaran/straw-oss/tree/master/docs/public/',
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
          alt: 'Straw Logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            docsPluginId: 'default',
            position: 'left',
            label: 'Documentation',
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
              {label: 'Quickstart Guide', to: '/docs/quickstart'},
              {label: 'Architecture', to: '/docs/architecture'},
              {label: 'Client SDKs', to: '/docs/sdk'},
              {label: 'CLI Reference', to: '/docs/cli'},
              {label: 'Egress Worker Setup', to: '/docs/egress_worker'},
            ],
          },
          {
            title: 'Run Straw',
            items: [
              {label: 'REST Request forwarding', to: '/docs/api/requests'},
              {label: 'Deployment', to: '/docs/deployment'},
              {label: 'System Operations', to: '/docs/operations'},
              {label: 'Security', to: '/docs/security'},
            ],
          },
          {
            title: 'More',
            items: [
              {label: 'GitHub', href: 'https://github.com/beremaran/straw-oss'},
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
