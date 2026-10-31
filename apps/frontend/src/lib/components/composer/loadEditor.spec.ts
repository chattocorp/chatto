import { beforeEach, expect, it, vi } from 'vitest';

const { markdown, visual } = vi.hoisted(() => ({ markdown: vi.fn(), visual: vi.fn() }));
vi.mock('./MarkdownEditor.svelte', markdown);
vi.mock('./TipTapEditor.svelte', visual);

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
});

it('shares pending imports, keeps the other editor lazy, and returns loaded code synchronously', async () => {
  const module = { default: () => {} };
  let resolve!: (value: typeof module) => void;
  markdown.mockImplementation(() => new Promise((done) => (resolve = done)));
  const { loadEditor } = await import('./loadEditor');
  const pending = loadEditor('markdown');
  expect(loadEditor('markdown')).toBe(pending);
  await vi.waitFor(() => expect(markdown).toHaveBeenCalledOnce());
  expect(visual).not.toHaveBeenCalled();
  resolve(module);
  const loaded = await pending;
  expect(loadEditor('markdown')).toBe(loaded);
  expect(loadEditor('markdown')).not.toBeInstanceOf(Promise);
});

it('does not retain a rejected import', async () => {
  visual.mockRejectedValue(new Error('Download failed'));
  const { loadEditor } = await import('./loadEditor');
  const first = loadEditor('visual');
  await expect(first).rejects.toThrow();
  const retry = loadEditor('visual');
  expect(retry).not.toBe(first);
  await expect(retry).rejects.toThrow();
});
