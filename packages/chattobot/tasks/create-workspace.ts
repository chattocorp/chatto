/** Create one Paseo worktree without starting an agent. The daemon owns its lifecycle. */
import type { PaseoClient, PaseoClientConfig } from '@getpaseo/client';
import { createPaseoClient } from '../paseo/client.ts';
import { awaitWithSignal, task, Type } from 'runling';
import { ConfigurationError } from '../settings.ts';
import { paseoSettings, type PaseoSettings } from '../paseo/settings.ts';

/** Model-selected metadata only. Repository, daemon, and base branch are host-owned. */
export const WorkspaceInput = Type.Object(
  {
    title: Type.String({ minLength: 1, maxLength: 120, pattern: '\\S' }),
    branchName: Type.String({
      minLength: 1,
      maxLength: 100,
      pattern: '^[a-z0-9]+(?:[/-][a-z0-9]+)*$'
    })
  },
  { additionalProperties: false }
);

/** Confirmed remote workspace identity, retained if subsequent agent launch fails. */
export const WorkspaceResult = Type.Object({
  id: Type.String(),
  name: Type.Union([Type.String(), Type.Null()]),
  directory: Type.Union([Type.String(), Type.Null()]),
  branchName: Type.String(),
  labelApplied: Type.Boolean({
    description:
      'Whether Paseo confirmed the chattobot label. A false value does not undo workspace creation.'
  })
});

/** Only the SDK operations this task needs; injectable for transport-free tests. */
export type WorkspaceClient = Pick<PaseoClient, 'connect' | 'close'> & {
  labelWorkspace(id: string): Promise<boolean>;
  workspaces: {
    create: (options: Parameters<PaseoClient['workspaces']['create']>[0]) => Promise<{
      id: string;
      name: string | null;
      directory: string | null;
    }>;
  };
};

/** Each invocation owns one connection. Abort/timeout closes it but cannot undo remote work.
 * Never retry creation automatically after an uncertain result. Raw SDK errors stay private. */
export function createWorkspaceTask(
  settings: PaseoSettings,
  createClient: (config: PaseoClientConfig) => WorkspaceClient = createPaseoClient
) {
  return task(
    { name: 'Create Paseo workspace', input: WorkspaceInput, result: WorkspaceResult },
    async (ctx, input) => {
      ctx.signal.throwIfAborted();
      const signal = AbortSignal.any([ctx.signal, AbortSignal.timeout(60_000)]);
      let client: WorkspaceClient | undefined;
      let creationSent = false;
      try {
        client = createClient({
          url: settings.url,
          password: settings.password,
          connectTimeoutMs: 10_000,
          reconnect: { enabled: false }
        });
        await awaitWithSignal(() => client!.connect(), signal);
        const workspace = await awaitWithSignal(() => {
          creationSent = true;
          return client!.workspaces.create({
            source: {
              kind: 'worktree',
              cwd: settings.repositoryPath,
              action: 'branch-off',
              refName: 'origin/main',
              branchName: input.branchName
            },
            title: input.title.trim()
          });
        }, signal);
        // Creation is already confirmed. A label failure must not cause another workspace
        // attempt or prevent the implementation from using this workspace.
        let labelApplied = false;
        try {
          labelApplied = await awaitWithSignal(() => client!.labelWorkspace(workspace.id), signal);
        } catch {
          ctx.signal.throwIfAborted();
        }
        return {
          id: workspace.id,
          name: workspace.name,
          directory: workspace.directory,
          branchName: input.branchName,
          labelApplied
        };
      } catch {
        throw new Error(
          creationSent
            ? 'Paseo workspace creation was not confirmed. It may have succeeded. Check Paseo before requesting another workspace; do not retry automatically.'
            : 'Could not connect to Paseo. Check the configured endpoint, credentials, and daemon availability.'
        );
      } finally {
        // Cleanup must not replace a confirmed creation result with an apparent failure.
        await client?.close().catch(() => {});
      }
    }
  );
}

/** Local operators can invoke this task with JSON input using their configured Paseo host. */
export default task(
  { name: 'Create Paseo workspace', input: WorkspaceInput, result: WorkspaceResult },
  (ctx, input) => {
    const settings = paseoSettings();
    if (!settings)
      throw new ConfigurationError('Set CHATTO_PASEO_URL and CHATTO_PASEO_REPOSITORY_PATH');
    return createWorkspaceTask(settings)(ctx, input);
  }
);
