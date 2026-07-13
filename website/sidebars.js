// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    {
      type: 'category',
      label: 'Learn',
      items: ['index', 'quickstart', 'architecture'],
    },
    {
      type: 'category',
      label: 'Use',
      items: ['api/requests', 'proxy-ingress', 'sdk', 'cli', 'egress_worker'],
    },
    {
      type: 'category',
      label: 'Operate',
      items: ['deployment', 'runtime-administration', 'highly-available-control', 'object-storage-receipts', 'operations', 'security', 'troubleshooting'],
    },
    {
      type: 'category',
      label: 'Reference',
      items: ['configuration', 'api/admin', 'api/receipts', 'components', 'compatibility', 'threat-model'],
    },
    {
      type: 'category',
      label: 'Contribute',
      items: ['development', 'test-strategy', 'release-checklist', 'documentation-policy', 'releases'],
    },
  ],
};

export default sidebars;
