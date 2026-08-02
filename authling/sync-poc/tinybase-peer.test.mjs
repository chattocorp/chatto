import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp, readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createInterface} from 'node:readline';
import test from 'node:test';

import {createMergeableStore} from 'tinybase/mergeable-store';
import {createCustomSynchronizer} from 'tinybase/synchronizers';

const undefinedMarker = '\uFFFC';

const stringify = (value) =>
  JSON.stringify(value, (_key, item) =>
    item === undefined ? undefinedMarker : item,
  );

const decodeLeaf = (stamp) => {
  if (Array.isArray(stamp) && stamp[0] === undefinedMarker) stamp[0] = undefined;
};

const decodeValues = (stamp) => {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] != 'object') return;
  Object.values(stamp[0]).forEach(decodeLeaf);
};

const decodeTables = (stamp) => {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] != 'object') return;
  for (const table of Object.values(stamp[0])) {
    if (!Array.isArray(table) || !table[0] || typeof table[0] != 'object') continue;
    for (const row of Object.values(table[0])) {
      if (!Array.isArray(row) || !row[0] || typeof row[0] != 'object') continue;
      Object.values(row[0]).forEach(decodeLeaf);
    }
  }
};

const decodeBody = (message, body, responseTo) => {
  if (message === 3 && Array.isArray(body)) {
    decodeTables(body[0]);
    decodeValues(body[1]);
  } else if (message === 0 && responseTo === 4 && Array.isArray(body)) {
    decodeTables(body[0]);
  } else if (message === 0 && responseTo === 7) {
    decodeValues(body);
  }
  return body;
};

class PeerProcess {
  constructor(statePath, peerCount = 1) {
    const executable = process.env.AUTHLING_TINYBASE_TEST_PEER;
    assert.ok(executable, 'AUTHLING_TINYBASE_TEST_PEER must be set');
    this.process = spawn(executable, ['-state', statePath, '-peers', String(peerCount)], {
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    this.clients = new Map();
    this.errors = '';
    this.process.stderr.setEncoding('utf8');
    this.process.stderr.on('data', (chunk) => (this.errors += chunk));
    this.process.once('exit', (code) => {
      if (code && this.errors) process.stderr.write(this.errors);
    });
    createInterface({input: this.process.stdout}).on('line', (line) => {
      const message = JSON.parse(line);
      const client = this.clients.get(message.clientId);
      if (client?.online) {
        const responseTo = client.pending.get(message.requestId);
        if (message.message === 0) client.pending.delete(message.requestId);
        client.receive(
          'authling',
          message.requestId,
          message.message,
          decodeBody(message.message, message.body, responseTo),
        );
      } else if (client) {
        client.inbound.push(message);
      }
    });
  }

  createClient(clientId, store, peer = 0) {
    const connection = {
      inbound: [],
      online: true,
      outbound: [],
      pending: new Map(),
      receive: () => {},
    };
    this.clients.set(clientId, connection);

    const synchronizer = createCustomSynchronizer(
      store,
      (_toClientId, requestId, message, body) => {
        const envelope = {peer, clientId, requestId, message, body};
        if (message !== 0 && requestId !== null) {
          connection.pending.set(requestId, message);
        }
        if (connection.online) {
          this.write(envelope);
        } else {
          connection.outbound.push(envelope);
        }
      },
      (receive) => (connection.receive = receive),
      () => this.clients.delete(clientId),
      2,
    );

    return {
      store,
      synchronizer,
      setOnline: (online) => {
        connection.online = online;
        if (!online) return;
        for (const message of connection.outbound.splice(0)) {
          this.write(message);
        }
        for (const message of connection.inbound.splice(0)) {
          const responseTo = connection.pending.get(message.requestId);
          if (message.message === 0) connection.pending.delete(message.requestId);
          connection.receive(
            'authling',
            message.requestId,
            message.message,
            decodeBody(message.message, message.body, responseTo),
          );
        }
      },
    };
  }

  write(message) {
    this.process.stdin.write(stringify(message) + '\n');
  }

  async stop() {
    this.process.stdin.end();
    const [code] = await new Promise((resolve) =>
      this.process.once('exit', (...result) => resolve(result)),
    );
    assert.equal(code, 0, this.errors);
  }
}

const waitFor = async (condition, description) => {
  const deadline = Date.now() + 3_000;
  while (!(await condition())) {
    if (Date.now() > deadline) {
      assert.fail(`timed out waiting for ${description}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
};

test('TinyBase 9.3 devices converge through a restarted Go peer', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'authling-tinybase-'));
  const statePath = join(directory, 'state.json');

  const deviceAStore = createMergeableStore('device-a');
  deviceAStore.setRow('servers', 'one', {
    name: 'First server',
    url: 'https://one.example',
  });
  deviceAStore.setValue('preferences', {
    nested: {__authling_tinybase_undefined: true},
    reserved: '\uFFFC',
  });
  const firstPeer = new PeerProcess(statePath);
  const firstDeviceA = firstPeer.createClient('device-a', deviceAStore);
  await firstDeviceA.synchronizer.startSync();
  await waitFor(
    async () => {
      try {
        return (await readFile(statePath, 'utf8')).includes('one.example');
      } catch {
        return false;
      }
    },
    'the peer to pull device A local state',
  );
  await firstDeviceA.synchronizer.destroy();
  await firstPeer.stop();

  const secondPeer = new PeerProcess(statePath);
  const deviceBStore = createMergeableStore('device-b');
  const deviceB = secondPeer.createClient('device-b', deviceBStore);
  await deviceB.synchronizer.startSync();
  assert.equal(deviceBStore.getCell('servers', 'one', 'name'), 'First server');
  assert.deepEqual(deviceBStore.getValue('preferences'), {
    nested: {__authling_tinybase_undefined: true},
    reserved: '\uFFFC',
  });

  deviceBStore.setRow('servers', 'two', {
    name: 'Second server',
    url: 'https://two.example',
  });

  const deviceA = secondPeer.createClient('device-a', deviceAStore);
  await deviceA.synchronizer.startSync();
  await waitFor(
    () => deviceAStore.getCell('servers', 'two', 'name') == 'Second server',
    'device A to receive device B data',
  );

  deviceA.setOnline(false);
  deviceAStore.setValue('theme', 'light');
  await new Promise((resolve) => setTimeout(resolve, 5));
  deviceBStore.setValue('theme', 'dark');
  await waitFor(
    () => deviceB.synchronizer.getSynchronizerStats().sends >= 2,
    'device B to send its preference',
  );
  deviceA.setOnline(true);
  await waitFor(
    () =>
      deviceAStore.getValue('theme') == deviceBStore.getValue('theme') &&
      deviceAStore.getValue('theme') == 'dark',
    'offline conflict to converge',
  );

  deviceBStore.delRow('servers', 'one');
  await waitFor(
    () => !deviceAStore.hasRow('servers', 'one'),
    'deletion tombstone to reach device A',
  );

  await deviceA.synchronizer.destroy();
  await deviceB.synchronizer.destroy();
  await secondPeer.stop();
});

test('TinyBase 9.3 clients converge after an OCC conflict between live peers', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'authling-tinybase-occ-'));
  const statePath = join(directory, 'state.json');
  const peerProcess = new PeerProcess(statePath, 2);

  const observerStore = createMergeableStore('observer');
  const observer = peerProcess.createClient('observer', observerStore, 1);
  await observer.synchronizer.startSync();

  const sourceStore = createMergeableStore('source');
  const source = peerProcess.createClient('source', sourceStore, 1);
  await source.synchronizer.startSync();
  await source.synchronizer.stopSync();

  const remoteStore = createMergeableStore('remote-writer');
  remoteStore.setValue('remote', 'winner');
  const remote = peerProcess.createClient('remote-writer', remoteStore, 0);
  await remote.synchronizer.startSync();
  await waitFor(
    async () => {
      try {
        return (await readFile(statePath, 'utf8')).includes('winner');
      } catch {
        return false;
      }
    },
    'the first peer write',
  );

  sourceStore.setValue('local', 'change');
  await source.synchronizer.startSync();

  await waitFor(
    () =>
      sourceStore.getValue('remote') === 'winner' &&
      sourceStore.getValue('local') === 'change' &&
      observerStore.getValue('remote') === 'winner' &&
      observerStore.getValue('local') === 'change',
    'the response source and observer to receive the durable winner',
  );

  await remote.synchronizer.destroy();
  await observer.synchronizer.destroy();
  await source.synchronizer.destroy();
  await peerProcess.stop();
});
