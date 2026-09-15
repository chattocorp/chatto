import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as highlighting from '$lib/codeHighlighting';
import JsonCode from './JsonCode.svelte';

vi.mock('$lib/codeHighlighting', { spy: true });

afterEach(() => vi.restoreAllMocks());

describe('JSON payload', () => {
  it('highlights JSON values and preserves whitespace and HTML-like text', async () => {
    const text =
      '{\n  "html": "<img src=x onerror=alert(1)>",\n  "values": [2, true, false, null]\n}\n';
    const { container } = render(JsonCode, { text });
    await expect.poll(() => container.querySelector('.hljs-attr')?.textContent).toBe('"html"');
    expect(container.querySelector('.hljs-string')?.textContent).toBe(
      '"<img src=x onerror=alert(1)>"'
    );
    expect(container.querySelector('.hljs-number')?.textContent).toBe('2');
    expect(
      [...container.querySelectorAll('.hljs-literal')].map((node) => node.textContent)
    ).toEqual(['true', 'false', 'null']);
    expect(container.querySelector('code')?.textContent).toBe(text);
    expect(container.querySelector('img')).toBeNull();
  });

  it('shows current plain text while loading and highlights only the current payload', async () => {
    await highlighting.ensureCodeLanguageLoaded('json');
    const pending = Promise.withResolvers<boolean>();
    vi.mocked(highlighting.ensureCodeLanguageLoaded)
      .mockReturnValueOnce(pending.promise)
      .mockReturnValueOnce(pending.promise);
    const rendered = render(JsonCode, { text: '{"old": 1}' });
    await expect
      .poll(() => rendered.container.querySelector('code')?.textContent)
      .toBe('{"old": 1}');
    await rendered.rerender({ text: '{"new": 2}' });
    expect(rendered.container.querySelector('code')?.textContent).toBe('{"new": 2}');
    expect(rendered.container.querySelector('.hljs-attr')).toBeNull();
    pending.resolve(true);
    await expect
      .poll(() => rendered.container.querySelector('.hljs-attr')?.textContent)
      .toBe('"new"');
    expect(rendered.container.querySelector('code')?.textContent).toBe('{"new": 2}');
  });

  it('keeps the payload readable when highlighting fails', async () => {
    await highlighting.ensureCodeLanguageLoaded('json');
    vi.spyOn(highlighting.lowlight, 'highlight').mockImplementation(() => {
      throw new Error('unavailable');
    });
    const text = '{"value": null}';
    const { container } = render(JsonCode, { text });
    await expect.poll(() => highlighting.lowlight.highlight).toHaveBeenCalled();
    expect(container.querySelector('code')?.textContent).toBe(text);
    expect(container.querySelector('.hljs-attr')).toBeNull();
  });
});
