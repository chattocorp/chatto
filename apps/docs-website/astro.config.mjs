// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { docsSiteUrl } from "./src/docsMetadata.ts";

// https://astro.build/config
export default defineConfig({
  site: docsSiteUrl,
  redirects: {
    "/getting-started/overview": "/getting-started/introduction",
    "/guides/deployment-read-this-first": "/guides/deployment/read-this-first",
    "/guides/binary": "/guides/deployment/binary",
    "/guides/dockercompose": "/guides/deployment/docker-compose",
    "/guides/kubernetes": "/guides/deployment/kubernetes",
    "/guides/community-structure": "/guides/operations/community-structure",
    "/guides/identity-login": "/guides/operations/identity-login",
    "/guides/permissions": "/guides/operations/permissions",
    "/guides/notifications-web-push": "/guides/operations/notifications-web-push",
    "/guides/privacy-erasure": "/guides/operations/privacy-erasure",
    "/guides/server-operations": "/guides/operations/server-operations",
    "/guides/backup-restore": "/guides/operations/backup-restore",
    "/guides/security": "/guides/operations/security",
    "/guides/operator-cli": "/guides/operations/operator-cli",
    "/guides/media-attachments": "/guides/infrastructure/media-attachments",
    "/guides/horizontal-scaling": "/guides/infrastructure/horizontal-scaling",
    "/guides/high-availability": "/guides/infrastructure/high-availability",
    "/guides/s3-storage": "/guides/infrastructure/s3-storage",
    "/guides/video-processing": "/guides/infrastructure/video-processing",
    "/guides/voice-calls": "/guides/infrastructure/voice-calls",
    "/guides/integrating-with-chatto": "/guides/integrations/chatto-api",
    "/guides/external-login-providers": "/guides/integrations/external-login-providers",
    "/guides/community-shields": "/guides/integrations/community-shields",
  },
  integrations: [
    starlight({
      title: "Chatto",
      customCss: ["./src/custom.css"],
      routeMiddleware: "./src/routeData.ts",
      components: {
        Banner: "./src/components/DocsBanner.astro",
        SiteTitle: "./src/components/DocsSiteTitle.astro",
        SocialIcons: "./src/components/SocialIcons.astro",
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/chattocorp/chatto",
        },
      ],
      sidebar: [
        {
          label: "Getting Started",
          items: [
            "getting-started/introduction",
            "getting-started/quick-start",
            "getting-started/faq",
          ],
        },
        {
          label: "Deployment",
          items: [
            "guides/deployment/read-this-first",
            "guides/deployment/binary",
            "guides/deployment/docker-compose",
            "guides/deployment/kubernetes",
            "reference/environment-variables",
          ],
        },
        {
          label: "Operating Your Server",
          items: [
            "guides/operations/community-structure",
            "guides/operations/identity-login",
            "guides/operations/permissions",
            "guides/operations/notifications-web-push",
            "guides/operations/security",
            "guides/operations/privacy-erasure",
            "guides/operations/server-operations",
            "guides/operations/search",
            "guides/operations/operator-cli",
            "guides/operations/backup-restore",
          ],
        },
        {
          label: "Scaling & Infrastructure",
          items: [
            "guides/infrastructure/media-attachments",
            "guides/infrastructure/horizontal-scaling",
            "guides/infrastructure/high-availability",
            "guides/infrastructure/s3-storage",
            "guides/infrastructure/video-processing",
            "guides/infrastructure/voice-calls",
          ],
        },
        {
          label: "Integrations",
          items: [
            "guides/integrations/chatto-api",
            "guides/integrations/api-compatibility",
            "guides/integrations/external-login-providers",
            "guides/integrations/community-shields",
            "guides/integrations/pocket-id",
          ],
        },
        {
          label: "Releases",
          items: ["releases/0-4-0", "releases/0-3-0", "releases/0-2-0"],
        },
        {
          label: "API Reference",
          items: [
            "reference/connectrpc-api",
            {
              label: "chatto.auth.v1",
              items: ["reference/connectrpc-api/external-identity-auth"],
            },
            {
              label: "chatto.discovery.v1",
              items: ["reference/connectrpc-api/server-discovery"],
            },
            {
              label: "chatto.api.v1",
              items: [
                "reference/connectrpc-api/assets",
                "reference/connectrpc-api/asset-uploads",
                "reference/connectrpc-api/messages",
                "reference/connectrpc-api/message-search",
                "reference/connectrpc-api/account",
                "reference/connectrpc-api/notification-preferences",
                "reference/connectrpc-api/notifications",
                "reference/connectrpc-api/push-notifications",
                "reference/connectrpc-api/roles",
                "reference/connectrpc-api/room-directory",
                "reference/connectrpc-api/rooms",
                "reference/connectrpc-api/server",
                "reference/connectrpc-api/threads",
                "reference/connectrpc-api/users",
                "reference/connectrpc-api/viewer",
                "reference/connectrpc-api/calls",
              ],
            },
            {
              label: "chatto.admin.v1",
              items: [
                "reference/connectrpc-api/admin-diagnostics",
                "reference/connectrpc-api/admin-event-log",
                "reference/connectrpc-api/admin-permissions",
                "reference/connectrpc-api/admin-roles",
                "reference/connectrpc-api/admin-room-layout",
                "reference/connectrpc-api/admin-server",
                "reference/connectrpc-api/admin-users",
              ],
            },
            {
              label: "chatto.realtime.v1",
              items: ["reference/connectrpc-api/realtime"],
            },
            "reference/connectrpc-api/types",
          ],
        },
      ],
    }),
  ],
});
