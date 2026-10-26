/** Use real Pi session storage and context hooks to detect snapshot accumulation. */
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test } from 'vitest';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import { threadContextExtension, type ThreadContext } from './thread-context.ts';

test('fresh snapshots reach requests but never enter saved history across turns and reopen', async () => {
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-context-'));
  try {
    const file = join(directory, 'session.jsonl');
    const runtime = await ModelRuntime.create({
      authPath: join(directory, 'auth.json'),
      modelsPath: null,
      refreshOnCreate: false,
      allowModelNetwork: false
    });
    const model = runtime.getModel('anthropic', 'claude-sonnet-4-6');
    if (!model) throw new Error('Test requires the built-in model catalog');
    const open = async (snapshot: ThreadContext, vision = true) => {
      const settingsManager = SettingsManager.inMemory();
      const resourceLoader = new DefaultResourceLoader({
        cwd: directory,
        agentDir: directory,
        settingsManager,
        noExtensions: true,
        noSkills: true,
        noPromptTemplates: true,
        noThemes: true,
        noContextFiles: true,
        extensionFactories: [threadContextExtension(snapshot)]
      });
      await resourceLoader.reload();
      return createAgentSession({
        cwd: directory,
        agentDir: directory,
        modelRuntime: runtime,
        model: vision ? model : { ...model, input: ['text'] },
        settingsManager,
        resourceLoader,
        tools: [],
        sessionManager: SessionManager.open(file, directory, directory)
      });
    };
    const manager = SessionManager.open(file, directory, directory);
    manager.appendMessage({
      role: 'user',
      content: JSON.stringify({
        currentMessages: [
          { id: 'first', text: 'first addressed batch' },
          { id: 'unchanged', text: 'unchanged addressed message' },
          { id: 'older', text: 'outside snapshot window' }
        ]
      }),
      timestamp: Date.now()
    });
    const first = await open({
      messages: [{ id: 'ordinary', author: 'human', text: 'snapshot version one' }],
      memoryDocuments: { global: ['old-global-memory'], users: { human: ['old-user-memory'] } },
      images: [
        { messageId: 'ordinary', attachmentId: 'image', data: 'AQID', mimeType: 'image/png' }
      ],
      olderContextOmitted: false
    });
    try {
      const request = await first.session.extensionRunner.emitContext(first.session.messages);
      expect(JSON.stringify(request)).toContain('snapshot version one');
      expect(JSON.stringify(request)).toContain('old-global-memory');
      expect(JSON.stringify(request)).toContain('old-user-memory');
      expect(JSON.stringify(request)).toContain('"type":"image"');
      expect(await readFile(file, 'utf8')).not.toContain('AQID');
      expect(await readFile(file, 'utf8')).not.toContain('old-global-memory');
      expect(JSON.stringify(first.session.messages)).not.toContain('snapshot version one');
      expect(await readFile(file, 'utf8')).not.toContain('snapshot version one');
      // A repeated transform (such as a retry) starts from history, not the previous request copy.
      expect(await first.session.extensionRunner.emitContext(first.session.messages)).toEqual(
        request
      );
    } finally {
      first.session.dispose();
    }

    const textOnly = await open(
      {
        messages: [],
        olderContextOmitted: false,
        images: [
          { messageId: 'ordinary', attachmentId: 'image', data: 'AQID', mimeType: 'image/png' }
        ]
      },
      false
    );
    try {
      const request = JSON.stringify(
        await textOnly.session.extensionRunner.emitContext(textOnly.session.messages)
      );
      expect(request).not.toContain('AQID');
      expect(request).toContain('cannot read images');
    } finally {
      textOnly.session.dispose();
    }
    const reopened = SessionManager.open(file, directory, directory);
    reopened.appendMessage({
      role: 'assistant',
      content: [{ type: 'text', text: 'Bot answer' }],
      api: 'anthropic-messages',
      provider: 'anthropic',
      model: model.id,
      usage: {
        input: 0,
        output: 0,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: 0,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
      },
      stopReason: 'stop',
      timestamp: Date.now()
    });
    const secondEntry = reopened.appendMessage({
      role: 'user',
      content: 'second addressed batch',
      timestamp: Date.now()
    });
    const second = await open({
      memoryDocuments: { global: ['new-global-memory'], users: { human: [] } },
      messages: [
        { id: 'ordinary', author: 'human', text: 'snapshot version two' },
        { id: 'first', text: 'edited first batch' },
        { id: 'unchanged', text: 'unchanged addressed message' },
        { id: 'reply', text: 'Bot answer', fromViewer: true },
        { id: 'human-copy', text: 'Bot answer', fromViewer: false }
      ],
      olderContextOmitted: true
    });
    try {
      const request = await second.session.extensionRunner.emitContext(second.session.messages);
      const text = JSON.stringify(request);
      expect(text).toContain('new-global-memory');
      expect(text).not.toContain('old-global-memory');
      expect(text).not.toContain('old-user-memory');
      expect(await readFile(file, 'utf8')).not.toContain('new-global-memory');
      expect(text).toContain('snapshot version two');
      expect(text).not.toContain('snapshot version one');
      expect(text.match(/snapshot version two/g)).toHaveLength(1);
      expect(text.match(/edited first batch/g)).toHaveLength(1);
      expect(text.match(/unchanged addressed message/g)).toHaveLength(1);
      expect(text.match(/outside snapshot window/g)).toHaveLength(1);
      expect(text).not.toContain('first addressed batch');
      // The bot reply is already in history; a human quoting it is a distinct message.
      expect(text.match(/Bot answer/g)).toHaveLength(2);
      expect(text).not.toContain('\\"reply\\"');
      expect(await second.session.extensionRunner.emitContext(second.session.messages)).toEqual(
        request
      );
      expect(second.session.sessionId).toBe(manager.getSessionId());
      expect(JSON.stringify(second.session.messages)).toContain('first addressed batch');
      expect(JSON.stringify(second.session.messages)).toContain('second addressed batch');
      expect(await readFile(file, 'utf8')).not.toContain('snapshot version');
    } finally {
      second.session.dispose();
    }
    const compacted = SessionManager.open(file, directory, directory);
    compacted.appendCompaction('Earlier conversation summary', secondEntry, 100);
    const third = await open({
      messages: [
        { id: 'first', text: 'edited first batch' },
        { id: 'reply', text: 'Bot answer', fromViewer: true }
      ],
      olderContextOmitted: false
    });
    try {
      const request = await third.session.extensionRunner.emitContext(third.session.messages);
      const text = JSON.stringify(request);
      expect(text).toContain('Earlier conversation summary');
      expect(text.match(/edited first batch/g)).toHaveLength(1);
      expect(text.match(/Bot answer/g)).toHaveLength(1);
      expect(JSON.stringify(third.session.messages)).not.toContain('Bot answer');
    } finally {
      third.session.dispose();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
