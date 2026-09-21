import {
  createAgentSession,
  DefaultResourceLoader,
  SessionManager,
} from "@earendil-works/pi-coding-agent";

const [, , extensionPath, temporaryDirectory] = process.argv;
if (extensionPath === undefined || temporaryDirectory === undefined) {
  throw new Error("Expected an extension path and temporary directory");
}

const resourceLoader = new DefaultResourceLoader({
  cwd: temporaryDirectory,
  agentDir: temporaryDirectory,
  additionalExtensionPaths: [extensionPath],
  noSkills: true,
  noPromptTemplates: true,
  noThemes: true,
  noContextFiles: true,
});
await resourceLoader.reload({ resolveProjectTrust: async () => true });

const loaded = resourceLoader.getExtensions();
const loadedTools = loaded.extensions.flatMap((extension) => [
  ...extension.tools.keys(),
]);
const { session } = await createAgentSession({
  model: {
    id: "integration-test",
    name: "Integration test",
    provider: "test",
    api: "openai-completions",
    baseUrl: "http://localhost",
    reasoning: false,
    input: ["text"],
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    contextWindow: 1_000,
    maxTokens: 100,
  },
  resourceLoader,
  sessionManager: SessionManager.inMemory(temporaryDirectory),
  tools: ["web_fetch"],
});

try {
  console.log(
    JSON.stringify({
      errors: loaded.errors,
      loadedTools,
      selectedTools: session.agent.state.tools.map(({ name }) => name),
    }),
  );
} finally {
  session.dispose();
}
