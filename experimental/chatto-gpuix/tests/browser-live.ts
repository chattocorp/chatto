/** Optional manual-browser integration check against two explicitly selected disposable local servers. */
import { createServer } from 'node:http';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';
import {
  check,
  click_test_id,
  find_test_id,
  finish,
  mount_headless,
  screenshot,
  wait_for
} from 'gpuix-svelte/test';
import App from '../src/App.svelte';
import { NativeSession } from '../src/session.svelte.ts';
import { openNativeState } from '../src/state.ts';
import { browserSignIn } from '../src/browser-auth.ts';
import metadata from './oauth-client.json' with { type: 'json' };

const firstOrigin = process.env.CHATTO_GPUIX_TEST_ORIGIN;
const secondOrigin = process.env.CHATTO_GPUIX_TEST_SECOND_ORIGIN;
for (const origin of [firstOrigin, secondOrigin])
  if (!origin || !['127.0.0.1', 'localhost', '[::1]'].includes(new URL(origin).hostname))
    throw new Error('Select two disposable loopback Chatto servers');
const base = fileURLToPath(new URL('../../../.context/', import.meta.url));
mkdirSync(base, { recursive: true });
const directory = mkdtempSync(join(base, 'native-browser-live-'));
process.env.CHATTO_GPUIX_CONFIG_DIR = directory;
const key = randomBytes(32);
const openState = () => openNativeState({ directory, key: async () => Buffer.from(key) });
const pendingFile = join(base, 'native-browser-url.json');
rmSync(pendingFile, { force: true });
const metadataHost = createServer((_request, response) => {
  response.setHeader('Content-Type', 'application/json');
  response.end(JSON.stringify({ ...metadata, client_id: clientId, client_uri: metadataOrigin }));
});
await new Promise<void>((resolve) => metadataHost.listen(0, '127.0.0.1', resolve));
const address = metadataHost.address();
if (!address || typeof address === 'string') throw new Error('No metadata listener');
const metadataOrigin = `http://127.0.0.1:${address.port}`;
const clientId = `${metadataOrigin}/oauth/gpuix-client.json`;
let opens = 0;
const options = {
  openState,
  clientId,
  password: async () => 'foobar123',
  authorize: async (origin: string, id: string, signal: AbortSignal) =>
    browserSignIn(origin, id, signal, async (url) => {
      opens++;
      writeFileSync(pendingFile, JSON.stringify({ url: url.href }), { mode: 0o600 });
    })
};
let session = new NativeSession(options);
let mounted: ReturnType<typeof mount_headless> | null = null;
try {
  await session.ready();
  session.origin = firstOrigin!;
  mounted = mount_headless(App, { props: { session }, width: 1100, height: 538 });
  await session.signInWithBrowser();
  if (!session.server) throw new Error('Browser sign-in did not register a server');
  check(
    'browser sign-in binds the public OAuth client',
    session.activeRegistration?.oauthClientId,
    clientId
  );
  const firstId = session.activeServerId;
  await session.server.ready();
  await wait_for(() => !!session.server?.roomList.rooms.length, {
    timeout: 15_000,
    label: 'first directory'
  });
  const firstRoom = session.server.roomList.rooms[0]!.id;
  session.select(firstRoom);
  const oldRefresh = session.activeRegistration?.refreshToken;
  await session.server.connection.apiConfig.renewBearerToken?.(true);
  check(
    'SDK rotation replaces the saved refresh credential',
    session.activeRegistration?.refreshToken !== oldRefresh
  );
  session.beginAdd();
  session.origin = secondOrigin!;
  session.login = 'alice';
  await session.signIn();
  const secondId = session.activeServerId;
  if (firstId === secondId || !session.server) throw new Error('Second server was not added');
  await session.server.ready();
  await wait_for(() => !!session.server?.roomList.rooms.length, {
    timeout: 15_000,
    label: 'second directory'
  });
  const secondRoom = session.server.roomList.rooms[0]!.id;
  session.select(secondRoom);
  check('two servers stay registered', session.servers.length, 2);
  session.activate(firstId);
  check('each server keeps its room selection', session.selected, firstRoom);
  session.moveServer(secondId, -1);
  check('gutter order can change', session.servers[0]?.id, secondId);
  await mounted.unmount();
  mounted = null;
  await session.close();
  session = new NativeSession(options);
  await session.ready();
  check(
    'both sessions restore from the encrypted file',
    session.servers.filter((server) => server.token).length,
    2
  );
  check('active server restores', session.activeServerId, firstId);
  check(
    'room and gutter order restore',
    session.selected === firstRoom && session.servers[0]?.id === secondId
  );
  check('restart does not open another browser', opens, 1);
  mounted = mount_headless(App, { props: { session }, width: 1100, height: 538 });
  await session.server?.ready();
  if (!find_test_id(`server-${firstId}`) || !find_test_id(`server-${secondId}`))
    throw new Error('The native gutter is missing a server');
  click_test_id(`server-${secondId}`);
  check('native gutter switches SDK servers', session.activeServerId, secondId);
  await session.server?.ready();
  session.select(secondRoom);
  if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
  const secondRefresh = session.activeRegistration?.refreshToken;
  session.activate(firstId);
  await session.signOut();
  check(
    'sign-out leaves the other server authenticated',
    session.servers.find((server) => server.id === secondId)?.refreshToken,
    secondRefresh
  );
  await session.removeServer(firstId);
  check(
    'removal affects only the selected server',
    session.servers.map((server) => server.id).join(','),
    secondId
  );
  await session.signOut();
  check(
    'encrypted state contains no plaintext refresh credential',
    readFileSync(join(directory, 'state.enc')).includes(Buffer.from(secondRefresh!)),
    false
  );
} finally {
  await mounted?.unmount();
  await session.close();
  key.fill(0);
  rmSync(pendingFile, { force: true });
  rmSync(directory, { recursive: true, force: true });
  await new Promise<void>((resolve) => metadataHost.close(() => resolve()));
}
finish('Native multi-server browser flow', 13);
