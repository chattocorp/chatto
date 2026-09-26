// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { docsSiteUrl } from './src/docsMetadata.ts';

// https://astro.build/config
export default defineConfig({
  site: docsSiteUrl,
  redirects: {
    '/getting-started/overview': '/getting-started/introduction',
    '/guides/deployment-read-this-first': '/guides/deployment/read-this-first',
    '/guides/binary': '/guides/deployment/binary',
    '/guides/dockercompose': '/guides/deployment/docker-compose',
    '/guides/kubernetes': '/guides/infrastructure/horizontal-scaling',
    '/guides/community-structure': '/guides/operations/community-structure',
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
    '/guides/operations/pinned-messages': '/guides/operations/community-structure',
    '/guides/operations/neighbors': '/guides/operations/community-structure',
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
            'getting-started/faq'
          ]
        },
        {
          label: 'Deployment',
          items: [
            'guides/deployment/read-this-first',
            'guides/deployment/docker-compose',
            'guides/deployment/binary',
            'guides/infrastructure/horizontal-scaling',
            'reference/environment-variables'
          ]
        },
        {
          label: 'Running a Server',
          items: [
            'guides/operations/community-structure',
            'guides/operations/permissions',
            'guides/operations/identity-login',
            'guides/integrations/external-login-providers',
            'guides/infrastructure/media-attachments',
            'guides/infrastructure/voice-calls',
            'guides/operations/search',
            'guides/operations/notifications-web-push',
            'guides/operations/server-operations',
            'guides/operations/backup-restore',
            'guides/operations/operator-cli',
            'guides/operations/security'
          ]
        },
        {
          label: 'Integrations',
          items: [
            'guides/integrations/chatto-api',
            'guides/integrations/bot-accounts',
            'guides/integrations/realtime-protocol',
            'guides/integrations/mcp',
            'guides/integrations/api-compatibility'
          ]
        },
        {
          label: 'How Chatto Works',
          items: ['how-chatto-works/architecture', 'how-chatto-works/encryption']
        },
        {
          label: 'Releases',
          items: ['releases/0-5-0', 'releases/0-4-0', 'releases/0-3-0', 'releases/0-2-0']
        },
        {
          label: 'API Reference',
          items: [
            'reference/connectrpc-api',
            {
              label: 'chatto.auth.v1',
              items: [
                'reference/connectrpc-api/external-identity-auth',
                'reference/connectrpc-api/server-setup',
                'reference/connectrpc-api/push-subscription-cleanup'
              ]
            },
            {
              label: 'chatto.discovery.v1',
              items: ['reference/connectrpc-api/server-discovery']
            },
            {
              label: 'chatto.api.v1',
              items: [
                'reference/connectrpc-api/assets',
                'reference/connectrpc-api/asset-uploads',
                'reference/connectrpc-api/bots',
                'reference/connectrpc-api/permissions',
                'reference/connectrpc-api/messages',
                'reference/connectrpc-api/message-search',
                'reference/connectrpc-api/account',
                'reference/connectrpc-api/notifications',
                'reference/connectrpc-api/push-notifications',
                'reference/connectrpc-api/roles',
                'reference/connectrpc-api/room-directory',
                'reference/connectrpc-api/rooms',
                'reference/connectrpc-api/server',
                'reference/connectrpc-api/threads',
                'reference/connectrpc-api/users',
                'reference/connectrpc-api/viewer',
                'reference/connectrpc-api/calls'
              ]
            },
            {
              label: 'chatto.admin.v1',
              items: [
                'reference/connectrpc-api/admin-invite-links',
                'reference/connectrpc-api/admin-oauth-clients',
                'reference/connectrpc-api/admin-diagnostics',
                'reference/connectrpc-api/admin-event-log',
                'reference/connectrpc-api/admin-permissions',
                'reference/connectrpc-api/admin-roles',
                'reference/connectrpc-api/admin-room-layout',
                'reference/connectrpc-api/admin-server',
                'reference/connectrpc-api/admin-users'
              ]
            },
            {
              label: 'chatto.realtime.v1',
              items: ['reference/connectrpc-api/realtime']
            },
            'reference/connectrpc-api/types'
          ]
        }
      ]
    })
  ]
});
