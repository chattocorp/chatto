/** Encrypted atomic native state. SDK authentication writes are durable before renewal requests. */
import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import {
  chmodSync,
  closeSync,
  constants,
  existsSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  realpathSync,
  renameSync,
  unlinkSync,
  writeFileSync
} from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import type {
  RegisteredServer,
  ServerAuthentication,
  ServerRegistryPersistence
} from '@chatto/client/server/registry';
import { stateEncryptionKey } from './keychain.ts';

/** Device-local view choices, independent of server protocol and credentials. */
export type NativeUiState = {
  /** Optional for vaults written before UI scaling was added. */
  uiScale?: number;
  activeServerId: string;
  rooms: Record<string, string>;
  order: string[];
};
type StateData = {
  version: 1;
  servers: RegisteredServer[];
  authentication: Record<string, ServerAuthentication>;
  ui: NativeUiState;
};
const empty = (): StateData => ({
  version: 1,
  servers: [],
  authentication: {},
  ui: { activeServerId: '', rooms: {}, order: [] }
});

/** Native durable store. Only successful disk writes become visible to SDK read-back checks. */
export class NativeState implements ServerRegistryPersistence {
  readonly shared = false;
  #closed = false;
  #closing: Promise<void> | null = null;
  constructor(
    private data: StateData,
    private flush: (data: StateData) => void,
    private release: () => Promise<void> = async () => {}
  ) {}
  readServers(): RegisteredServer[] {
    return structuredClone(this.data.servers);
  }
  readAuthentication(id: string): ServerAuthentication | undefined {
    return this.data.authentication[id] ? { ...this.data.authentication[id] } : undefined;
  }
  writeAuthentication(id: string, authentication: ServerAuthentication): boolean {
    this.commit({
      ...this.data,
      authentication: { ...this.data.authentication, [id]: { ...authentication } }
    });
    return true;
  }
  writeServers(servers: RegisteredServer[]): void {
    const ids = new Set(servers.map((server) => server.id));
    this.commit({
      ...this.data,
      servers: structuredClone(servers),
      authentication: Object.fromEntries(
        Object.entries(this.data.authentication).filter(([id]) => ids.has(id))
      )
    });
  }
  get ui(): NativeUiState {
    return structuredClone(this.data.ui);
  }
  setUi(ui: NativeUiState): void {
    this.commit({ ...this.data, ui: structuredClone(ui) });
  }
  private commit(data: StateData): void {
    if (this.#closed) throw new Error('Native session storage is closed');
    this.flush(data);
    this.data = data;
  }
  /** Stop writes, discard decrypted data, and release exclusive process access. */
  close(): Promise<void> {
    if (this.#closing) return this.#closing;
    this.#closed = true;
    this.data = empty();
    this.#closing = this.release();
    return this.#closing;
  }
}

/** In-memory host store for deterministic tests; production uses openNativeState. */
export function memoryState(): NativeState {
  return new NativeState(empty(), () => {});
}

/** Only one process may renew a saved session; lockf releases its OS lock on exit or stdin EOF. */
async function acquireLock(
  file: string
): Promise<{ release: () => Promise<void>; held: () => boolean }> {
  const fd = openSync(file, constants.O_CREAT | constants.O_RDWR | constants.O_NOFOLLOW, 0o600);
  closeSync(fd);
  return new Promise((resolve, reject) => {
    const child = spawn(
      '/usr/bin/lockf',
      ['-k', '-t', '0', file, '/bin/sh', '-c', 'printf "READY\\n"; cat >/dev/null'],
      { stdio: ['pipe', 'pipe', 'ignore'] }
    );
    let acquired = false;
    const exited = new Promise<void>((done) => child.once('exit', () => done()));
    const timer = setTimeout(() => {
      child.kill();
      reject(new Error('Could not lock native session storage'));
    }, 5000);
    child.once('error', () => {
      clearTimeout(timer);
      reject(new Error('Could not lock native session storage'));
    });
    child.once('exit', () => {
      clearTimeout(timer);
      if (!acquired)
        reject(new Error('Another Chatto GPUix instance is using this state directory'));
    });
    child.stdout.once('data', () => {
      acquired = true;
      clearTimeout(timer);
      resolve({
        release: async () => {
          child.stdin.end();
          await exited;
        },
        held: () => child.exitCode === null && child.signalCode === null && !child.killed
      });
    });
  });
}

/** Open the vault, using a Keychain-held AES key and a private, process-locked directory. */
export async function openNativeState(
  options: {
    directory?: string;
    key?: (directory: string, exists: boolean) => Promise<Buffer>;
  } = {}
): Promise<NativeState> {
  const dir =
    options.directory ??
    process.env.CHATTO_GPUIX_CONFIG_DIR ??
    join(homedir(), 'Library', 'Application Support', 'Chatto GPUix');
  if (existsSync(dir) && lstatSync(dir).isSymbolicLink())
    throw new Error('Invalid native state directory');
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  chmodSync(dir, 0o700);
  const file = join(dir, 'state.enc');
  const lease = await acquireLock(join(dir, 'state.lock'));
  let key: Buffer | null = null;
  try {
    const exists = existsSync(file);
    if (
      exists &&
      (!lstatSync(file).isFile() ||
        lstatSync(file).isSymbolicLink() ||
        lstatSync(file).size > 4 * 1024 * 1024)
    )
      throw new Error();
    key = await (options.key ?? stateEncryptionKey)(realpathSync(dir), exists);
    if (key.length !== 32) throw new Error();
    let data = empty();
    if (exists) {
      const bytes = readFileSync(file);
      if (bytes.subarray(0, 4).toString() !== 'CGX1' || bytes.length < 32) throw new Error();
      const decipher = createDecipheriv('aes-256-gcm', key, bytes.subarray(4, 16));
      decipher.setAuthTag(bytes.subarray(16, 32));
      const decoded: unknown = JSON.parse(
        Buffer.concat([decipher.update(bytes.subarray(32)), decipher.final()]).toString('utf8')
      );
      data = validateState(decoded);
    }
    const secret = key;
    const flush = (next: StateData) => {
      if (!lease.held()) throw new Error('Exclusive native session access was lost');
      const iv = randomBytes(12);
      const cipher = createCipheriv('aes-256-gcm', secret, iv);
      const encrypted = Buffer.concat([cipher.update(JSON.stringify(next)), cipher.final()]);
      const bytes = Buffer.concat([Buffer.from('CGX1'), iv, cipher.getAuthTag(), encrypted]);
      if (bytes.length > 4 * 1024 * 1024)
        throw new Error('Native session state exceeds its size limit');
      const temp = join(dir, `.state-${crypto.randomUUID()}`);
      let fd: number | undefined;
      try {
        fd = openSync(
          temp,
          constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW,
          0o600
        );
        writeFileSync(fd, bytes);
        fsyncSync(fd);
        closeSync(fd);
        fd = undefined;
        renameSync(temp, file);
        const directory = openSync(dir, constants.O_RDONLY);
        try {
          fsyncSync(directory);
        } finally {
          closeSync(directory);
        }
      } catch {
        if (fd !== undefined) closeSync(fd);
        try {
          unlinkSync(temp);
        } catch {
          /* No temporary file remains after a successful rename. */
        }
        throw new Error('Could not save native sessions securely');
      }
    };
    return new NativeState(data, flush, async () => {
      secret.fill(0);
      await lease.release();
    });
  } catch {
    key?.fill(0);
    await lease.release();
    throw new Error(
      'Could not open saved sessions securely. Check Keychain access and the state directory.'
    );
  }
}

function validateState(value: unknown): StateData {
  if (!value || typeof value !== 'object') throw new Error();
  const data = value as Record<string, unknown>;
  if (
    data.version !== 1 ||
    !Array.isArray(data.servers) ||
    data.servers.length > 100 ||
    !data.authentication ||
    typeof data.authentication !== 'object' ||
    !data.ui ||
    typeof data.ui !== 'object'
  )
    throw new Error();
  const ui = data.ui as Record<string, unknown>;
  if (
    (ui.uiScale !== undefined &&
      (typeof ui.uiScale !== 'number' ||
        !Number.isFinite(ui.uiScale) ||
        ui.uiScale < 0.5 ||
        ui.uiScale > 2)) ||
    typeof ui.activeServerId !== 'string' ||
    !Array.isArray(ui.order) ||
    !ui.order.every((id) => typeof id === 'string') ||
    !ui.rooms ||
    typeof ui.rooms !== 'object' ||
    !Object.values(ui.rooms).every((room) => typeof room === 'string')
  )
    throw new Error();
  // The SDK validates catalogue/session records again before restoring a client.
  for (const auth of Object.values(data.authentication)) {
    if (!auth || typeof auth !== 'object') throw new Error();
    const fields = auth as Record<string, unknown>;
    for (const name of ['token', 'refreshToken', 'oauthClientId', 'refreshRequestId'])
      if (fields[name] !== null && typeof fields[name] !== 'string') throw new Error();
    for (const name of ['accessTokenExpiresAt', 'refreshTokenExpiresAt', 'reauthRequiredAt'])
      if (
        fields[name] !== null &&
        (typeof fields[name] !== 'number' || !Number.isFinite(fields[name]))
      )
        throw new Error();
  }
  return value as StateData;
}
