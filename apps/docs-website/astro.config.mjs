// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { docsSiteUrl } from './src/docsMetadata.ts';

// https://astro.build/config
export default defineConfig({
  site: docsSiteUrl,
  // Paseo's route points to one allocated port. Fail instead of switching ports.
  vite: { server: { strictPort: Boolean(process.env.PASEO_PORT) } },
  redirects: {
    '/how-chatto-works/architecture': '/guides/deployment/read-this-first',
    '/how-chatto-works/encryption': '/guides/operations/security#encryption-and-data-erasure',
    '/getting-started/overview': '/getting-started/introduction',
    '/guides/deployment-read-this-first': '/guides/deployment/read-this-first',
    '/guides/binary': '/guides/deployment/binary',
    '/guides/dockercompose': '/guides/deployment/docker-compose',
    '/guides/kubernetes': '/guides/infrastructure/horizontal-scaling',
    '/guides/community-structure': '/guides/operations/permissions',
    '/guides/identity-login': '/guides/operations/identity-login',
    '/guides/permissions': '/guides/operations/permissions',
    '/guides/notifications-web-push': '/guides/operations/notifications-web-push',
    '/guides/privacy-erasure': '/guides/operations/security',
    '/guides/server-operations': '/guides/operations/server-operations',
    '/guides/backup-restore': '/guides/operations/backup-restore',
    '/guides/security': '/guides/operations/security',
    '/guides/operator-cli': '/guides/operations/operator-cli',
    '/guides/media-attachments': '/guides/infrastructure/media-attachments',
    '/guides/horizontal-scaling': '/guides/infrastructure/horizontal-scaling',
    '/guides/high-availability': '/guides/infrastructure/horizontal-scaling',
    '/guides/s3-storage': '/guides/infrastructure/media-attachments',
    '/guides/video-processing': '/guides/infrastructure/media-attachments',
    '/guides/voice-calls': '/guides/infrastructure/voice-calls',
    '/guides/integrating-with-chatto': '/guides/integrations/chatto-api',
    '/guides/external-login-providers': '/guides/integrations/external-login-providers',
    '/guides/community-shields': '/guides/operations/server-operations',
    '/guides/infrastructure/high-availability': '/guides/infrastructure/horizontal-scaling',
    '/guides/deployment/kubernetes': '/guides/infrastructure/horizontal-scaling',
    '/guides/operations/pinned-messages': '/guides/operations/permissions',
    '/guides/operations/neighbors': '/guides/operations/permissions#neighbors',
    '/guides/integrations/community-shields': '/guides/operations/server-operations',
    '/guides/operations/cross-server-connections': '/guides/operations/identity-login',
    '/guides/integrations/pocket-id': '/guides/integrations/external-login-providers',
    '/guides/infrastructure/s3-storage': '/guides/infrastructure/media-attachments',
    '/guides/infrastructure/video-processing': '/guides/infrastructure/media-attachments',
    '/guides/operations/privacy-erasure': '/guides/operations/security',
    '/guides/integrations/human-oauth': '/guides/integrations/chatto-api',
    '/guides/integrations/grafana': '/guides/integrations/bot-accounts',
    '/guides/integrations/realtime-typescript': '/guides/integrations/realtime-protocol'
  },
  integrations: [
    starlight({
      title: 'Chatto',
      customCss: ['./src/custom.css'],
      routeMiddleware: './src/routeData.ts',
      components: {
        Banner: './src/components/DocsBanner.astro',
        SiteTitle: './src/components/DocsSiteTitle.astro',
        SocialIcons: './src/components/SocialIcons.astro'
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/chattocorp/chatto'
        }
      ],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            'getting-started/introduction',
            'getting-started/quick-start',
            'getting-started/install-pwa',
            'getting-started/faq'
          ]
        },
        {
          label: 'Self-Hosting',
          items: [
            'guides/deployment/read-this-first',
            'guides/deployment/docker-compose',
            'guides/deployment/binary',
            'reference/environment-variables',
            'guides/operations/identity-login',
            'guides/integrations/external-login-providers',
            'guides/infrastructure/media-attachments',
            'guides/infrastructure/voice-calls',
            'guides/operations/search',
            'guides/operations/notifications-web-push'
          ]
        },
        {
          label: 'Operations',
          collapsed: true,
          items: [
            'guides/operations/server-operations',
            'guides/operations/backup-restore',
            'guides/operations/security',
            'guides/operations/permissions',
            'guides/operations/operator-cli',
            'guides/infrastructure/horizontal-scaling'
          ]
        },
        {
          label: 'Releases',
          collapsed: true,
          items: ['releases/0-5-0', 'releases/0-4-0', 'releases/0-3-0', 'releases/0-2-0']
        },
        {
          label: 'Developers',
          collapsed: true,
          items: [
            'guides/integrations/chatto-api',
            'guides/integrations/bot-accounts',
            'guides/integrations/realtime-protocol',
            'guides/integrations/mcp',
            'guides/integrations/api-compatibility',
            'reference/message-syntax',
            'reference/bot-webhooks',
            'reference/mcp',
            {
              label: 'API Reference',
              collapsed: true,
              items: [{ autogenerate: { directory: 'reference/connectrpc-api' } }]
            }
          ]
        }
      ]
    })
  ]
});
