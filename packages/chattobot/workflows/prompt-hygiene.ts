/** Keep host details out of the prompts of agents that have no file tools. */
import { defineAgentExtension } from 'runling/agents';

/** Pi appends the agent's working directory to every system prompt. Agents without file tools do
 * not need it, and it would send a host path to the model provider. */
export const withoutWorkingDirectory = defineAgentExtension((pi) => {
  pi.on('before_agent_start', (event) => ({
    systemPrompt: event.systemPrompt
      .replace(/\n*Current working directory: [^\n]*\n?/g, '\n')
      .trimEnd()
  }));
});
