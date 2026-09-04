import { runTestBot, type TestBotConfig } from "./bot.js";

function requiredEnvironment(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required`);
  return value;
}

const config: TestBotConfig = {
  serverUrl: requiredEnvironment("CHATTO_TEST_BOT_SERVER_URL"),
  apiKeyFile: requiredEnvironment("CHATTO_TEST_BOT_API_KEY_FILE"),
  stateFile: requiredEnvironment("CHATTO_TEST_BOT_STATE_FILE"),
  ai: {
    provider: process.env.CHATTO_TEST_BOT_AI_PROVIDER ?? "faux",
    model: process.env.CHATTO_TEST_BOT_AI_MODEL,
    fauxResponse: process.env.CHATTO_TEST_BOT_AI_FAUX_RESPONSE,
  },
};

const controller = new AbortController();
for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.once(signal, () => controller.abort());
}

try {
  await runTestBot(config, controller.signal);
} catch (error) {
  console.error(
    JSON.stringify({
      component: "test_bot",
      status: "fatal",
      error: error instanceof Error && error.name ? error.name : "UnknownError",
    }),
  );
  process.exitCode = 1;
}
