/** SDK connection with the workspace-label RPC, which is not yet on the workspace helper. */
import { createPaseoApi, type PaseoClientConfig } from '@getpaseo/client';
import { DaemonClient } from '@getpaseo/client/internal/daemon-client';

/** Own one SDK connection and its subscriptions. Keep the internal SDK dependency here
 * until its public workspace helper exposes labels. Labeling adds only our fixed label. */
export function createPaseoClient(config: PaseoClientConfig) {
  const daemon = new DaemonClient({
    ...config,
    clientId: config.clientId ?? `chattobot-${crypto.randomUUID()}`,
    clientType: 'cli'
  });
  const api = createPaseoApi(daemon);
  return {
    ...api,
    connect: () => daemon.connect(),
    close: async () => {
      try {
        await api.dispose();
      } finally {
        await daemon.close();
      }
    },
    labelWorkspace: async (workspaceId: string) => {
      const result = await daemon.setWorkspaceLabel({
        workspaceId,
        label: { name: 'chattobot', color: 'emerald' },
        assigned: true
      });
      return result.workspaceLabels.includes('chattobot');
    }
  };
}
