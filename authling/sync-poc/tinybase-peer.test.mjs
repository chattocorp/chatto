import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp, readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createInterface} from 'node:readline';
import test from 'node:test';

import {createMergeableStore} from 'tinybase/mergeable-store';
import {createCustomSynchronizer} from 'tinybase/synchronizers';

const undefinedMarker = {'__authling_tinybase_undefined': true};

const stringify = (value) =>
  JSON.stringify(value, (_key, item) =>
    item === undefined ? undefinedMarker : item,
  );

const parse = (value) =>
  JSON.parse(value, (_key, item) =>
    item &&
    typeof item == 'object' &&
    Object.keys(item).length == 1 &&
    item.__authling_tinybase_undefined === true
      ? undefined
      : item,
  );

class PeerProcess {
  constructor(statePath) {
    const executable = process.env.AUTHLING_TINYBASE_TEST_PEER;
    assert.ok(executable, 'AUTHLING_TINYBASE_TEST_PEER must be set');
    this.process = spawn(executable, ['-state', statePath], {
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    this.clients = new Map();
    this.errors = '';
    this.process.stderr.setEncoding('utf8');
    this.process.stderr.on('data', (chunk) => (this.errors += chunk));
    createInterface({input: this.process.stdout}).on('line', (line) => {
      const message = parse(line);
      const client = this.clients.get(message.clientId);
      if (client?.online) {
        client.receive(
          'authling',
          message.requestId,
          message.message,
          message.body,
        );
      } else if (client) {
        client.inbound.push(message);
      }
    });
  }

  createClient(clientId, store) {
    const connection = {
      inbound: [],
      online: true,
      outbound: [],
      receive: () => {},
    };
    this.clients.set(clientId, connection);

    const synchronizer = createCustomSynchronizer(
      store,
      (_toClientId, requestId, message, body) => {
        const envelope = {clientId, requestId, message, body};
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
          connection.receive(
            'authling',
            message.requestId,
            message.message,
            message.body,
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
