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
          items: ["getting-started/introduction", "getting-started/quick-start"],
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
                "reference/connectrpc-api/identity",
                "reference/connectrpc-api/rooms-and-messages",
                "reference/connectrpc-api/notifications",
                "reference/connectrpc-api/administration",
                "reference/connectrpc-api/types",
                "reference/connectrpc-api/realtime",
              ],
            },
            "reference/environment-variables",
          ],
        },
      ],
    }),
  ],
});
