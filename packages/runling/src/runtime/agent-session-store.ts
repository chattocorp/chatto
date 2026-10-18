/** Private sessions that can outlive a workflow. Run-owned sessions should use
 * agentSessionFile instead. Shared sessions expire after 30 days without use. */
import { mkdir, readdir, rm, stat, utimes } from 'node:fs/promises';
import { resolve } from 'node:path';
import { log } from './log.ts';

const RETENTION_MS = 30 * 24 * 60 * 60 * 1000;
const ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
// Configuration reload must not permit two turns to open the same session at once.
const STATE = Symbol.for('runling.agentSessionLeases');
const active = ((globalThis as typeof globalThis & { [STATE]?: Set<string> })[STATE] ??=
  new Set<string>());

/** An exclusive session lease. Dispose after disposing the agent that writes the file. */
export interface AgentSessionLease extends AsyncDisposable {
  readonly id: string;
  readonly file: string;
}

/** Filesystem storage for sessions resumed across separate runs. The directory is host
 * configuration, never model input. Opening a session also purges expired, inactive files
 * in that directory. This does not recover execution or authorize a new agent turn. */
export class AgentSessionStore {
  readonly directory: string;

  constructor(directory: string) {
    this.directory = resolve(directory);
  }

  /** Keep application scopes separate. The key is a single host-owned path component. */
  scope(key: string): AgentSessionStore {
    if (!/^[a-zA-Z0-9_-]+$/.test(key)) throw new TypeError('Invalid agent session scope');
    return new AgentSessionStore(resolve(this.directory, key));
  }

  /** Open a new session, or explicitly resume an existing one. Reject duplicate IDs and
   * concurrent use. Callers must dispose the lease even when agent creation fails. */
  async open({
    id = crypto.randomUUID(),
    resume = false
  }: { id?: string; resume?: boolean } = {}): Promise<AgentSessionLease> {
    if (!ID.test(id)) throw new TypeError('Invalid agent session ID');
    const file = resolve(this.directory, `${id}.jsonl`);
    if (active.has(file)) throw new Error(`Agent session ${id} is already in use`);
    active.add(file);
    try {
      await mkdir(this.directory, { recursive: true, mode: 0o700 });
      const info = await stat(file).catch((error: NodeJS.ErrnoException) => {
        if (error.code === 'ENOENT') return undefined;
        throw error;
      });
      if (resume && !info) throw new Error(`There is no agent session ${id}`);
      if (!resume && info) throw new Error(`Agent session ${id} exists already`);
      // An explicit resume renews the session before cleanup. Open leases never expire.
      if (info) await utimes(file, new Date(), new Date());
      await this.prune();
      let closed = false;
      return {
        id,
        file,
        async [Symbol.asyncDispose]() {
          if (closed) return;
          closed = true;
          try {
            await utimes(file, new Date(), new Date());
          } catch (error) {
            if ((error as NodeJS.ErrnoException).code !== 'ENOENT')
              log.error('Could not update agent session activity');
          } finally {
            active.delete(file);
          }
        }
      };
    } catch (error) {
      active.delete(file);
      throw error;
    }
  }

  /** Remove inactive session files older than 30 days. Unrelated files and subdirectories
   * are preserved. Failures are logged and retried on the next open or explicit prune. */
  async prune(): Promise<void> {
    try {
      const entries = await readdir(this.directory, { withFileTypes: true });
      for (const entry of entries) {
        if (!entry.isFile() || !entry.name.endsWith('.jsonl') || !ID.test(entry.name.slice(0, -6)))
          continue;
        const file = resolve(this.directory, entry.name);
        const info = await stat(file);
        if (!active.has(file) && info.mtimeMs < Date.now() - RETENTION_MS) {
          active.add(file);
          try {
            await rm(file, { force: true });
          } finally {
            active.delete(file);
          }
        }
      }
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT')
        log.error('Could not clean up shared agent sessions');
    }
  }
}
