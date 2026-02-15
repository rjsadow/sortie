import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Sortie',
  description: 'Self-hosted application launcher for teams',
  base: '/docs/',
  outDir: './dist',

  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/' },
      { text: 'Admin', link: '/admin/' },
      { text: 'Developer', link: '/developer/' },
    ],

    sidebar: [
      {
        text: 'User Guide',
        items: [
          { text: 'Getting Started', link: '/guide/' },
          { text: 'Access Control', link: '/guide/access-control' },
          { text: 'Templates', link: '/guide/templates' },
          { text: 'Sessions', link: '/guide/sessions' },
          { text: 'Session Sharing', link: '/guide/session-sharing' },
          { text: 'Session Recording', link: '/guide/recording' },
        ],
      },
      {
        text: 'Administration',
        items: [
          { text: 'Overview', link: '/admin/' },
          { text: 'Deployment', link: '/admin/deployment' },
          { text: 'Kubernetes', link: '/admin/kubernetes' },
          { text: 'Reverse Proxy', link: '/admin/reverse-proxy' },
          { text: 'Data Persistence', link: '/admin/data-persistence' },
          { text: 'Session Recording', link: '/admin/recording' },
          { text: 'Disaster Recovery', link: '/admin/disaster-recovery' },
          { text: 'Network Egress', link: '/admin/network-egress' },
        ],
      },
      {
        text: 'Developer',
        items: [
          { text: 'Overview', link: '/developer/' },
          { text: 'Development', link: '/developer/development' },
          { text: 'Plugin System', link: '/developer/plugin-system' },
          { text: 'Architecture', link: '/developer/architecture' },
          { text: 'API Reference', link: '/developer/api-reference' },
        ],
      },
      {
        text: 'Decisions',
        items: [
          { text: 'ADR Index', link: '/decisions/' },
          { text: 'ADR-0001: Streaming Protocol', link: '/decisions/0001-streaming-protocol' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/rjsadow/sortie' },
    ],

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/rjsadow/sortie/edit/main/docs-site/:path',
    },
  },

  head: [
    ['link', { rel: 'icon', href: '/docs/favicon.ico' }],
  ],
})
