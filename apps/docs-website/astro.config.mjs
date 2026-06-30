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
                  label: "chatto.api.v1",
                  items: [
                    "reference/connectrpc-api/server-discovery",
                    "reference/connectrpc-api/server",
                    "reference/connectrpc-api/viewer",
                    "reference/connectrpc-api/account",
                    "reference/connectrpc-api/user-directory",
                    "reference/connectrpc-api/member-directory",
                    "reference/connectrpc-api/room-directory",
                    "reference/connectrpc-api/rooms",
                    "reference/connectrpc-api/room-timeline",
                    "reference/connectrpc-api/messages",
                    "reference/connectrpc-api/attachments",
                    "reference/connectrpc-api/reactions",
                    "reference/connectrpc-api/read-state",
                    "reference/connectrpc-api/threads",
                    "reference/connectrpc-api/link-previews",
                    "reference/connectrpc-api/calls",
                    "reference/connectrpc-api/notification-preferences",
                    "reference/connectrpc-api/notifications",
                    "reference/connectrpc-api/push-notifications",
                  ],
                },
                {
                  label: "chatto.admin.v1",
                  items: [
                    "reference/connectrpc-api/admin-server",
                    "reference/connectrpc-api/admin-room-layout",
                    "reference/connectrpc-api/admin-members",
                    "reference/connectrpc-api/admin-roles",
                    "reference/connectrpc-api/admin-permissions",
                    "reference/connectrpc-api/admin-diagnostics",
                    "reference/connectrpc-api/admin-event-log",
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
