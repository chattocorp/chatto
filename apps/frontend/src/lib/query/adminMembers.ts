import type { AdminMember, AdminRoleSummary } from '$lib/api-client/adminUsers';
import { adminQueryKeys } from './admin';
import { queryClient } from './client';

export type AdminMemberBatch = { users: AdminMember[]; roles: AdminRoleSummary[] };

/** Private row snapshots include role labels so both share the same privacy lifetime. */
type RowSnapshot = { member: AdminMember | null; roles: AdminRoleSummary[] };

export function adminMemberRowKey(serverId: string, scope: string, userId: string) {
  return [...adminQueryKeys.membersRoot(serverId, { queryScope: scope }), 'row', userId] as const;
}

/** Coalesce cache misses across admin pages. TanStack owns session-scoped
 * freshness and cancellation; batch completion never writes outside its query. */
export function createAdminMemberLoader(
  serverId: string,
  scope: string,
  load: (ids: string[], signal: AbortSignal) => Promise<AdminMemberBatch>
) {
  let pending: Array<{
    id: string;
    signal: AbortSignal;
    resolve: (row: RowSnapshot) => void;
    reject: (error: unknown) => void;
  }> = [];

  async function flush(): Promise<void> {
    const batch = pending;
    pending = [];
    for (let offset = 0; offset < batch.length; offset += 100) {
      const chunk = batch.slice(offset, offset + 100).filter((entry) => !entry.signal.aborted);
      if (!chunk.length) continue;
      const controller = new AbortController();
      const cancel = () => {
        if (chunk.every((entry) => entry.signal.aborted)) controller.abort();
      };
      for (const entry of chunk) entry.signal.addEventListener('abort', cancel);
      try {
        const result = await load(
          chunk.map((entry) => entry.id),
          controller.signal
        );
        const users = new Map(result.users.map((member) => [member.id, member]));
        for (const entry of chunk) {
          entry.resolve({ member: users.get(entry.id) ?? null, roles: result.roles });
        }
      } catch (error) {
        for (const entry of chunk) entry.reject(error);
      } finally {
        for (const entry of chunk) entry.signal.removeEventListener('abort', cancel);
      }
    }
  }

  return async (ids: string[], signal?: AbortSignal): Promise<AdminMemberBatch> => {
    signal?.throwIfAborted();
    const rows = await Promise.all(
      [...new Set(ids)].map((id) =>
        queryClient.fetchQuery({
          queryKey: adminMemberRowKey(serverId, scope, id),
          retry: false,
          queryFn: ({ signal }) =>
            new Promise<RowSnapshot>((resolve, reject) => {
              pending.push({ id, signal, resolve, reject });
              if (pending.length === 1) queueMicrotask(() => void flush());
            })
        })
      )
    );
    signal?.throwIfAborted();
    return {
      users: rows.flatMap(({ member }) => (member && !member.deleted ? [member] : [])),
      roles: rows.at(-1)?.roles ?? []
    };
  };
}
