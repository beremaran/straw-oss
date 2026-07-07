// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Straw',
  tagline: 'Distributed HTTP/HTTPS egress proxy control plane',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://beremaran.github.io',
  baseUrl: '/straw/',

  organizationName: 'beremaran',
  projectName: 'straw',

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
          editUrl: 'https://github.com/beremaran/straw/tree/master/docs/public/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      /** @type {import('@docusaurus/plugin-content-docs').Options} */
      ({
        id: 'internal',
        path: '../docs/planning',
        routeBasePath: 'internal',
        sidebarPath: './sidebars-internal.js',
        editUrl: 'https://github.com/beremaran/straw/tree/master/docs/planning/',
      }),
    ],
  ],

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
            label: 'SDK & API Docs',
          },
          {
            type: 'docSidebar',
            sidebarId: 'internalSidebar',
            docsPluginId: 'internal',
            position: 'left',
            label: 'Internal Docs',
          },
          {
            href: 'https://github.com/beremaran/straw',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              {label: 'Quickstart', to: '/docs/quickstart'},
              {label: 'Internal Architecture', to: '/internal/'},
            ],
          },
          {
            title: 'More',
            items: [
              {label: 'GitHub', href: 'https://github.com/beremaran/straw'},
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Straw.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
      },
    }),
};

export default config;
