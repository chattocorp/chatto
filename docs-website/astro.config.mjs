// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// https://astro.build/config
export default defineConfig({
  redirects: {
    "/getting-started/overview": "/getting-started/quick-start",
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
          items: ["getting-started/quick-start"],
        },
        {
          label: "Deployment",
          items: ["guides/binary", "guides/dockercompose", "guides/kubernetes"],
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
          label: "Reference",
          items: [
            {
              label: "ConnectRPC API",
              items: [
                "reference/connectrpc-api",
                "reference/connectrpc-api/notification-preferences-service",
                "reference/connectrpc-api/read-state-service",
                "reference/connectrpc-api/room-timeline-service",
                "reference/connectrpc-api/server-service",
                "reference/connectrpc-api/thread-service",
              ],
            },
            "reference/environment-variables",
          ],
        },
      ],
    }),
  ],
});
