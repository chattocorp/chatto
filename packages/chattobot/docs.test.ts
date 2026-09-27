import { expect, test, vi } from 'vitest';
import { AWESOME_CHATTO_HOME, DEV_DOCS_HOME, createPageCache, fetchDocsPage } from './docs.ts';

const html = (body: string) =>
  new Response(`<html><head><title>Chatto guide</title></head><body>${body}</body></html>`, {
    headers: { 'content-type': 'text/html; charset=utf-8' }
  });

test('returns readable content, canonical request URL, and same-site links without executing assets', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(
      html(
        '<nav><a href="/guide/">Guide</a></nav><main><h1>Hello &amp; welcome</h1><p>Read this.</p><script>secret script</script><a href="https://other.example">External</a></main>'
      )
    );
  const page = await fetchDocsPage('https://docs.chatto.run/#top', undefined, request);
  expect(page).toMatchObject({
    title: 'Chatto guide',
    url: 'https://docs.chatto.run/',
    truncated: false,
    links: [{ title: 'Guide', url: 'https://docs.chatto.run/guide/' }]
  });
  expect(page.text).toContain('Hello & welcome\n');
  expect(page.text).not.toContain('secret script');
  expect(request).toHaveBeenCalledOnce();
  expect(request.mock.calls[0]![1]).toMatchObject({ redirect: 'manual', credentials: 'omit' });
});

test.each([
  'https://evil.example/',
  'http://docs.chatto.run/',
  'https://docs.chatto.run.evil.example/',
  'https://secret@docs.chatto.run/',
  'https://docs.chatto.run/?secret=value',
  'https://docs.chatto.run:444/',
  'file:///etc/passwd',
  'https://dev-docs.chatto.run.evil.example/',
  'https://github.com/nickk-/awesome-chatto',
  'https://raw.githubusercontent.com/nickk-/awesome-chatto-evil/README.md',
  'https://raw.githubusercontent.com/nickk-/awesome-chatto/../other/HEAD/README.md',
  'https://raw.githubusercontent.com/nickk-/awesome-chatto/%2e%2e/other/HEAD/README.md',
  'https://raw.githubusercontent.com/nickk-/awesome-chatto/HEAD/README.md?token=secret'
])('rejects %s before fetching', async (url) => {
  const request = vi.fn<typeof fetch>();
  await expect(fetchDocsPage(url, undefined, request)).rejects.toThrow();
  expect(request).not.toHaveBeenCalled();
});

test('checks redirect targets and follows allowed relative redirects', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(new Response(null, { status: 302, headers: { location: '/guide/' } }))
    .mockResolvedValueOnce(html('<main>Guide</main>'));
  expect((await fetchDocsPage('/', undefined, request)).url).toBe('https://docs.chatto.run/guide/');
  request
    .mockReset()
    .mockResolvedValue(
      new Response(null, { status: 302, headers: { location: 'https://evil.example/' } })
    );
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('Only HTTPS');
  expect(request).toHaveBeenCalledOnce();
});

test('bounds redirect loops, body bytes, and returned text', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockImplementation(
      async () => new Response(null, { status: 302, headers: { location: '/' } })
    );
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('redirect limit');
  expect(request).toHaveBeenCalledTimes(5);
  request.mockResolvedValue(html('x'.repeat(512_001)));
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('size limit');
  request.mockResolvedValue(html(`<main>${'x'.repeat(31_000)}</main>`));
  const page = await fetchDocsPage('/', undefined, request);
  expect(page.text).toHaveLength(30_000);
  expect(page.truncated).toBe(true);
});

test('rejects failed or non-HTML responses', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(new Response('no', { status: 404 }));
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('unavailable');
  request.mockResolvedValue(new Response('binary', { headers: { 'content-type': 'image/png' } }));
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('unexpected format');
  request.mockResolvedValue(new Response('# Text', { headers: { 'content-type': 'text/plain' } }));
  await expect(fetchDocsPage('/', undefined, request)).rejects.toThrow('unexpected format');
});

test.each(['cancel', 'timeout'])('stops an in-flight request on %s', async (mode) => {
  vi.useFakeTimers();
  vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) => {
    expect(ms).toBe(15_000);
    const controller = new AbortController();
    setTimeout(() => controller.abort(), ms);
    return controller.signal;
  });
  try {
    const controller = new AbortController();
    const request = vi.fn<typeof fetch>().mockImplementation(async (_url, init) => {
      expect(init!.signal).not.toBe(controller.signal);
      return new Promise((_resolve, reject) =>
        init!.signal!.addEventListener('abort', () => reject(new Error('aborted')))
      );
    });
    const result = fetchDocsPage('/', controller.signal, request);
    const rejected = expect(result).rejects.toThrow('cancelled');
    if (mode === 'cancel') controller.abort();
    else await vi.advanceTimersByTimeAsync(15_000);
    await rejected;
  } finally {
    vi.restoreAllMocks();
    vi.useRealTimers();
  }
});

test('reads development docs and lists links to both documentation sites', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(
      html(
        '<main><a href="/beta/">Beta</a><a href="https://docs.chatto.run/guide/">Stable</a><a href="https://other.example/">Other</a></main>'
      )
    );
  const page = await fetchDocsPage(DEV_DOCS_HOME, undefined, request);
  expect(page.url).toBe('https://dev-docs.chatto.run/');
  expect(page.links).toEqual([
    { title: 'Beta', url: 'https://dev-docs.chatto.run/beta/' },
    { title: 'Stable', url: 'https://docs.chatto.run/guide/' }
  ]);
  expect(request.mock.calls[0]![1]!.headers).toMatchObject({ accept: 'text/html' });
});

test('reads the Awesome Chatto list as Markdown and lists only allowed links', async () => {
  const markdown = [
    '# Awesome Chatto',
    '',
    '- [Bot](https://github.com/example/bot), MIT <script>kept as text</script>',
    '- [Early discovery](Early%20Server%20Discovery.md)',
    '- [Docs](https://docs.chatto.run/)'
  ].join('\n');
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(
      new Response(markdown, { headers: { 'content-type': 'text/plain; charset=utf-8' } })
    );
  const page = await fetchDocsPage(AWESOME_CHATTO_HOME, undefined, request);
  expect(page).toMatchObject({
    url: AWESOME_CHATTO_HOME,
    title: 'Awesome Chatto',
    truncated: false,
    links: [
      {
        title: 'Early discovery',
        url: 'https://raw.githubusercontent.com/nickk-/awesome-chatto/HEAD/Early%20Server%20Discovery.md'
      },
      { title: 'Docs', url: 'https://docs.chatto.run/' }
    ]
  });
  expect(page.text).toContain('[Bot](https://github.com/example/bot)');
  expect(request.mock.calls[0]![1]!.headers).toMatchObject({
    accept: 'text/plain, text/markdown'
  });
  request.mockResolvedValue(html('<main>GitHub page</main>'));
  await expect(fetchDocsPage(AWESOME_CHATTO_HOME, undefined, request)).rejects.toThrow(
    'unexpected format'
  );
});

test('the page cache reuses successful reads until they expire', async () => {
  let time = 0;
  const page = (url: string) => ({ url, title: 'T', text: 'x', truncated: false, links: [] });
  const fetchPage = vi
    .fn(async (url: string) => page(url))
    .mockRejectedValueOnce(new Error('unavailable'));
  const read = createPageCache(fetchPage, { ttlMs: 100, maxEntries: 2, now: () => time });
  await expect(read('a')).rejects.toThrow('unavailable');
  await read('a');
  await read('a');
  expect(fetchPage).toHaveBeenCalledTimes(2);
  time = 150;
  await read('a');
  expect(fetchPage).toHaveBeenCalledTimes(3);
  await read('b');
  await read('c');
  await read('a');
  expect(fetchPage).toHaveBeenCalledTimes(6);
});
