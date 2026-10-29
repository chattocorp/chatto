/** Verify Pi transcript restoration in a fresh process without calling a model provider. */
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { expect, test } from 'vitest';
import { SessionManager } from '@earendil-works/pi-coding-agent';
import { transcriptName } from './agent.ts';
import { hasTranscript } from './messages.ts';

test('a fresh process restores the same Pi session and its model context', async () => {
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-resume-'));
  try {
    const identity = ['server', 'bot', 'room', 'thread'];
    const file = join(directory, transcriptName(identity));
    expect(await hasTranscript(directory, identity)).toBe(false);
    const original = SessionManager.open(file, directory, directory);
    original.appendThinkingLevelChange('high');
    original.appendMessage({
      role: 'user',
      content: 'Remember the synthetic test word: marmalade.',
      timestamp: Date.now()
    });
    expect(await hasTranscript(directory, identity)).toBe(true);
    expect(await hasTranscript(directory, ['server', 'other-bot', 'room', 'thread'])).toBe(false);
    const script = `
      import { SessionManager, ModelRuntime, SettingsManager, DefaultResourceLoader, createAgentSession } from '@earendil-works/pi-coding-agent';
      const [file, directory] = process.argv.slice(1);
      const settingsManager = SettingsManager.inMemory();
      const resourceLoader = new DefaultResourceLoader({ cwd: directory, agentDir: directory, settingsManager,
        noExtensions: true, noSkills: true, noPromptTemplates: true, noThemes: true, noContextFiles: true });
      await resourceLoader.reload();
      const modelRuntime = await ModelRuntime.create({ authPath: directory + '/auth.json', modelsPath: null,
        refreshOnCreate: false, allowModelNetwork: false });
      const model = modelRuntime.getModel('anthropic', 'claude-sonnet-4-6');
      if (!model) throw new Error('Synthetic restoration test needs a built-in model');
      const { session } = await createAgentSession({ cwd: directory, agentDir: directory, settingsManager,
        resourceLoader, modelRuntime, model, thinkingLevel: 'low', tools: [], sessionManager: SessionManager.open(file, directory, directory) });
      try { console.log(JSON.stringify({ sessionId: session.sessionId, messages: session.messages, thinkingLevel: session.thinkingLevel })); }
      finally { session.dispose(); }
    `;
    const { stdout } = await promisify(execFile)(
      process.execPath,
      ['--input-type=module', '-e', script, file, directory],
      {
        timeout: 15000,
        cwd: fileURLToPath(new URL('../', import.meta.url)),
        env: { ...process.env, PI_OFFLINE: '1' }
      }
    );
    const restored = JSON.parse(stdout);
    expect(restored.sessionId).toBe(original.getSessionId());
    expect(restored.thinkingLevel).toBe('low');
    expect(restored.messages).toEqual(original.buildSessionContext().messages);
    expect(restored.messages).toHaveLength(1);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}, 20000);
