/** Exercise connector restart, credential isolation, and process cleanup with real children. */
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test, vi } from 'vitest';
import { runTunnel } from './tunnel.ts';
import { parseAnnouncements } from './announcement-settings.ts';

function fixture(code: string) {
  const directory = mkdtempSync(join(tmpdir(), 'botto-tunnel-'));
  const command = join(directory, 'connector.cjs');
  const receipt = join(directory, 'receipt.json');
  writeFileSync(
    command,
    `#!${process.execPath}\nconst fs=require('node:fs');\nconst receipt=${JSON.stringify(receipt)};\n${code}`,
    { mode: 0o700 }
  );
  return { directory, command, receipt };
}

test('passes only connector credentials, restarts after exit, and reaps on abort', async () => {
  const f = fixture(`
const previous=fs.existsSync(receipt)?JSON.parse(fs.readFileSync(receipt)):null;
fs.writeFileSync(receipt,JSON.stringify({pid:process.pid,count:(previous?.count??0)+1,token:process.env.TUNNEL_TOKEN,args:process.argv.slice(2),leaked:process.env.BOTTO_TEST_PRIVATE_KEY??null}));
if(!previous) process.exit(1);
setInterval(()=>{},1000);
`);
  const controller = new AbortController();
  vi.stubEnv('BOTTO_TEST_PRIVATE_KEY', 'must-not-reach-connector');
  const running = runTunnel({ command: f.command, token: 'synthetic-token' }, controller.signal);
  try {
    await expect
      .poll(() => JSON.parse(readFileSync(f.receipt, 'utf8')).count, { timeout: 4000 })
      .toBe(2);
    const receipt = JSON.parse(readFileSync(f.receipt, 'utf8'));
    expect(receipt).toMatchObject({ token: 'synthetic-token', leaked: null });
    expect(receipt.args).toEqual(['tunnel', '--no-autoupdate', '--loglevel', 'error', 'run']);
    controller.abort();
    await running;
    expect(() => process.kill(receipt.pid, 0)).toThrow();
    expect(JSON.parse(readFileSync(f.receipt, 'utf8')).count).toBe(2);
  } finally {
    controller.abort();
    await running;
    vi.unstubAllEnvs();
    rmSync(f.directory, { recursive: true, force: true });
  }
});

test(
  'forces termination if the connector ignores graceful shutdown',
  { timeout: 10_000 },
  async () => {
    const f = fixture(
      `process.on('SIGTERM',()=>{}); fs.writeFileSync(receipt,JSON.stringify({pid:process.pid})); setInterval(()=>{},1000);`
    );
    const controller = new AbortController();
    const running = runTunnel({ command: f.command, token: 'synthetic' }, controller.signal);
    try {
      await expect.poll(() => JSON.parse(readFileSync(f.receipt, 'utf8')).pid).toBeGreaterThan(0);
      const { pid } = JSON.parse(readFileSync(f.receipt, 'utf8'));
      controller.abort();
      await running;
      expect(() => process.kill(pid, 0)).toThrow();
    } finally {
      controller.abort();
      await running;
      rmSync(f.directory, { recursive: true, force: true });
    }
  }
);

test('missing executable does not prevent cancellation during restart backoff', async () => {
  const controller = new AbortController();
  const running = runTunnel(
    { command: '/does-not-exist/botto-cloudflared', token: 'synthetic' },
    controller.signal
  );
  setTimeout(() => controller.abort(), 50);
  await running;
});

test('connector settings are optional and resolve local secret placeholders', () => {
  const github = {
    enabled: true,
    repository: 'owner/repo',
    room_id: 'room',
    webhook_secret: 's'.repeat(32),
    cloudflare_tunnel_token: '${TOKEN}'
  };
  expect(parseAnnouncements({ github }, { TOKEN: 'synthetic' })?.tunnel).toEqual({
    command: 'cloudflared',
    token: 'synthetic'
  });
  expect(() => parseAnnouncements({ github }, {})).toThrow('Invalid announcements.github');
  expect(() =>
    parseAnnouncements(
      {
        github: {
          ...github,
          cloudflare_tunnel_token: undefined,
          cloudflared_command: 'cloudflared'
        }
      },
      {}
    )
  ).toThrow('Invalid announcements.github');
});
