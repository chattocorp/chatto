/** Botto owns conversation naming policy; Runling owns the isolated description task. */
import { createDescription, log, type TaskContext } from 'runling';
import type { Settings } from './settings.ts';

/** Prepare a best-effort naming operation. The parent must join it before its turn ends. */
export function createTitleWriter(settings: Settings) {
  const describe = createDescription({
    name: 'Describe conversation',
    model: settings.titleModel,
    thinkingLevel: settings.titleReasoningLevel,
    instructions:
      'Write a short conversation title, at most 120 characters, in the language of the supplied messages. Describe the topic, not progress. Even a short question deserves a title. Omit the description for greetings or content with no meaningful topic. Do not include personal details, names, identifiers, or secrets. Do not answer the messages.'
  });
  return async (ctx: TaskContext, text: string, signal: AbortSignal): Promise<void> => {
    if (!ctx.run || ctx.run.title || !text.trim()) return;
    try {
      const title = await describe({ ...ctx, signal }, text.slice(0, 8000));
      signal.throwIfAborted();
      if (title && !ctx.run.title) ctx.run.setTitle(title);
    } catch {
      // Model errors can contain private request data. Keep the failure generic.
      if (!signal.aborted) log.warn('Conversation title unavailable');
    }
  };
}
