import { agent } from "runling/agents";
import webFetchExtension from "../../extensions/web-fetch.ts";
import type { ChattoAgentFactory } from "./agent-text.ts";

/** Demo-specific agent setup, separate from the conversation's message flow. */
export function createWebChatAgent({
  directory,
  model = "openai-codex/gpt-5.6-luna",
  createAgent = agent,
}: {
  directory: string;
  model?: string;
  createAgent?: ChattoAgentFactory;
}) {
  return createAgent({
    // Pi uses a directory for setup; the tool list controls model capabilities.
    cwd: directory,
    model,
    thinkingLevel: "medium",
    output: "text",
    tools: ["web_fetch"],

    // Register this tool explicitly instead of discovering local resources.
    // Tool restrictions are not an OS sandbox.
    extensions: [{ name: "runling-web-fetch", factory: webFetchExtension }],
    resources: {
      extensions: false,
      skills: false,
      promptTemplates: false,
      themes: false,
      contextFiles: false,
    },
    instructions: [
      "You are a helpful chat bot. Keep replies concise and respond to incoming messages.",
      "Your plain text is posted to the user immediately. Use it for brief progress updates when useful.",
      "Reply naturally in plain text. Use web_fetch to read public URLs when useful and cite sources in your reply. You have no file or shell tools.",
    ],
  });
}
