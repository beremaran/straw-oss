// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    'index',
    'quickstart',
    'architecture',
    'configuration',
    {
      type: 'category',
      label: 'Use Straw',
      items: ['api/requests', 'sdk', 'cli', 'egress_worker'],
    },
    {
      type: 'category',
      label: 'Run Straw',
      items: ['deployment', 'runtime-administration', 'operations', 'security', 'troubleshooting'],
    },
    'development',
  ],
};

export default sidebars;
