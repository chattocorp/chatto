/** Keep host details out of the prompts of agents that have no file tools. */
import { defineAgentExtension } from 'runling/agents';

/** Pi ends every system prompt with a `<cwd>` section that holds the agent's working directory.
 * Agents without file tools do not need it, and it would send a host path to the model provider.
 * Pi 1.0 writes the section as `<cwd>\n/path\n</cwd>`. */
export const withoutWorkingDirectory = defineAgentExtension((pi) => {
  pi.on('before_agent_start', (event) => ({
    systemPrompt: event.systemPrompt.replace(/\n*<cwd>\n[^\n]*\n<\/cwd>/g, '').trimEnd()
  }));
});
