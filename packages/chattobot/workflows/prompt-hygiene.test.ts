import { expect, test } from 'vitest';
import type { AgentExtensionAPI } from 'runling/agents';
import { withoutWorkingDirectory } from './prompt-hygiene.ts';

test('removes the working directory section from the system prompt', async () => {
  let handler!: (event: { systemPrompt: string }) => { systemPrompt: string };
  const factory =
    typeof withoutWorkingDirectory === 'function'
      ? withoutWorkingDirectory
      : withoutWorkingDirectory.factory;
  await factory({
    on(_name: string, registered: typeof handler) {
      handler = registered;
    }
  } as unknown as AgentExtensionAPI);
  // The format of Pi 1.0's structured system prompt.
  const prompt = 'You are terse.\n\n<addendum>\nBe kind.\n</addendum>\n\n<cwd>\n/srv/bot\n</cwd>';
  expect(handler({ systemPrompt: prompt }).systemPrompt).toBe(
    'You are terse.\n\n<addendum>\nBe kind.\n</addendum>'
  );
});
