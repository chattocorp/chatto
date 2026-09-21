import { defineConfig } from "vitest/config";
import { fileURLToPath } from "node:url";

export default defineConfig({
  resolve: {
    alias: [
      {
        find: /^runling\/agents$/,
        replacement: fileURLToPath(new URL("./src/runtime/agents/index.ts", import.meta.url)),
      },
      {
        find: /^runling$/,
        replacement: fileURLToPath(
          new URL("./src/runtime/index.ts", import.meta.url),
        ),
      },
      {
        find: /^runling\/git$/,
        replacement: fileURLToPath(
          new URL("./src/runtime/git.ts", import.meta.url),
        ),
      },
      {
        find: /^runling\/web$/,
        replacement: fileURLToPath(
          new URL("./src/runtime/web-config.ts", import.meta.url),
        ),
      },
    ],
  },
  test: {
    include: ["**/*.test.ts"],
    exclude: ["**/node_modules/**", "**/dist/**", "**/.context/**"],
    testTimeout: 15000,
    fileParallelism: false,
  },
});
