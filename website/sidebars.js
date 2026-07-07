// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    'index',
    'quickstart',
    'sdk',
    'cli',
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/auth',
        'api/requests',
        'api/config',
        'api/telemetry',
        'api/admin',
      ],
    },
    {
      type: 'category',
      label: 'Deployment & Operations',
      items: [
        'egress_worker',
        'operations',
      ],
    },
  ],
};

export default sidebars;
