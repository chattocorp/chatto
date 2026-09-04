import { Agent } from "@earendil-works/pi-agent-core";
import {
  contentText,
  createModels,
  fauxAssistantMessage,
  fauxProvider,
  type FauxProviderHandle,
  type Api,
  type Model,
  type Models,
} from "@earendil-works/pi-ai";
import { builtinModels } from "@earendil-works/pi-ai/providers/all";

const MAXIMUM_REPLY_LENGTH = 8_000;

const SYSTEM_PROMPT = `You are TestBot, a helpful AI participant in a Chatto thread.
Answer the latest human message using the preceding thread messages as context.
Be concise unless the user asks for detail. Use Markdown when it helps.
Do not repeat the @test_bot mention. Do not claim that you used tools or took actions.
Messages labeled Assistant are your earlier replies. Other labels identify people only within this prompt.`;

/** AI model configuration for TestBot. */
export interface TestBotAIConfig {
  provider: string;
  model?: string;
  fauxResponse?: string;
}

/** A small, tool-free text responder backed by Pi. */
export interface AIResponder {
  provider: string;
  model: string;
  respond(prompt: string, signal: AbortSignal): Promise<string>;
}

function responseText(agent: Agent): string {
  let message;
  for (let index = agent.state.messages.length - 1; index >= 0; index--) {
    const candidate = agent.state.messages[index];
    if (candidate?.role === "assistant") {
      message = candidate;
      break;
    }
  }
  if (!message || message.role !== "assistant") {
    throw new Error("AI response did not contain an assistant message");
  }
  if (message.stopReason === "error" || message.stopReason === "aborted") {
    throw new Error("AI response did not complete successfully");
  }
  const text = contentText(message.content).trim();
  if (!text) throw new Error("AI response was empty");
  if (text.length <= MAXIMUM_REPLY_LENGTH) return text;
  return `${text.slice(0, MAXIMUM_REPLY_LENGTH - 1).trimEnd()}…`;
}

function responder(
  models: Models,
  model: Model<Api>,
  faux: FauxProviderHandle | undefined,
  fauxResponse: string | undefined,
): AIResponder {
  return {
    provider: model.provider,
    model: model.id,
    async respond(prompt, signal): Promise<string> {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      if (faux) {
        faux.appendResponses([
          fauxAssistantMessage(
            fauxResponse ??
              "I am running with Pi's local faux provider. Configure an AI provider and model to enable generated replies.",
          ),
        ]);
      }
      const agent = new Agent({
        initialState: {
          systemPrompt: SYSTEM_PROMPT,
          model,
          tools: [],
          messages: [],
        },
        streamFn: models.streamSimple.bind(models),
      });
      const abort = () => agent.abort();
      signal.addEventListener("abort", abort, { once: true });
      try {
        await agent.prompt(prompt);
        return responseText(agent);
      } finally {
        signal.removeEventListener("abort", abort);
      }
    },
  };
}

/** Create a configured Pi responder without giving the model any tools. */
export async function createAIResponder(
  config: TestBotAIConfig,
): Promise<AIResponder> {
  if (config.provider === "faux") {
    const faux = fauxProvider({ tokensPerSecond: 0 });
    const models = createModels();
    models.setProvider(faux.provider);
    return responder(models, faux.getModel(), faux, config.fauxResponse);
  }

  if (!config.model) {
    throw new Error("CHATTO_TEST_BOT_AI_MODEL is required for a real provider");
  }
  const models = builtinModels();
  const model = models.getModel(config.provider, config.model);
  if (!model) throw new Error("configured AI provider or model is unknown");
  if (!(await models.getAuth(model))) {
    throw new Error("configured AI provider does not have credentials");
  }
  return responder(models, model, undefined, undefined);
}
