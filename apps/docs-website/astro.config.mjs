// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// https://astro.build/config
export default defineConfig({
  redirects: {
    "/getting-started/overview": "/getting-started/introduction",
  },
  integrations: [
    starlight({
      title: "Chatto",
      customCss: ["./src/custom.css"],
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
          ],
        },
        {
          label: "Deployment",
          items: [
            "guides/deployment-read-this-first",
            "guides/binary",
            "guides/dockercompose",
            "guides/kubernetes",
          ],
        },
        {
          label: "Guides",
          items: [
            "guides/horizontal-scaling",
            "guides/high-availability",
            "guides/s3-storage",
            "guides/video-processing",
            "guides/voice-calls",
            "guides/external-login-providers",
            "guides/backup-restore",
            "guides/operator-cli",
            "guides/security",
            "guides/permissions",
          ],
        },
        {
          label: "Releases",
          items: ["releases/0-4-0", "releases/0-3-0", "releases/0-2-0"],
        },
        {
          label: "Reference",
          items: [
            {
              label: "API Reference",
              items: [
                "reference/connectrpc-api",
                {
                  label: "chatto.auth.v1",
                  items: [
                    "reference/connectrpc-api/external-identity-auth",
                  ],
                },
                {
                  label: "chatto.discovery.v1",
                  items: [
                    "reference/connectrpc-api/server-discovery",
                  ],
                },
                {
                  label: "chatto.api.v1",
                  items: [
                    "reference/connectrpc-api/link-previews",
                    "reference/connectrpc-api/messages",
                    "reference/connectrpc-api/account",
                    "reference/connectrpc-api/notification-preferences",
                    "reference/connectrpc-api/notifications",
                    "reference/connectrpc-api/push-notifications",
                    "reference/connectrpc-api/roles",
                    "reference/connectrpc-api/room-directory",
                    "reference/connectrpc-api/room-members",
                    "reference/connectrpc-api/rooms",
                    "reference/connectrpc-api/server-members",
                    "reference/connectrpc-api/server",
                    "reference/connectrpc-api/threads",
                    "reference/connectrpc-api/user-directory",
                    "reference/connectrpc-api/viewer",
                    "reference/connectrpc-api/calls",
                  ],
                },
                {
                  label: "chatto.admin.v1",
                  items: [
                    "reference/connectrpc-api/admin-diagnostics",
                    "reference/connectrpc-api/admin-event-log",
                    "reference/connectrpc-api/admin-users",
                    "reference/connectrpc-api/admin-permissions",
                    "reference/connectrpc-api/admin-roles",
                    "reference/connectrpc-api/admin-room-layout",
                    "reference/connectrpc-api/admin-server",
                  ],
                },
                {
                  label: "chatto.realtime.v1",
                  items: ["reference/connectrpc-api/realtime"],
                },
                "reference/connectrpc-api/types",
              ],
            },
            "reference/environment-variables",
          ],
        },
      ],
    }),
  ],
});
