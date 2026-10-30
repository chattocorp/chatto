import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { AccountCreationPolicy } from '@chatto/api-types/api/v1/server_pb';
import { createChattoClient, type ConnectAPIConfig } from '@chatto/client/api/connect';

/** Admission choices editable by a server manager. */
export type RegistrationAdmission = 'open' | 'invite_only';

function admission(value: AccountCreationPolicy | undefined): RegistrationAdmission {
  if (value === AccountCreationPolicy.OPEN) return 'open';
  if (value === AccountCreationPolicy.INVITE_ONLY) return 'invite_only';
  throw new Error('The server returned an unsupported registration policy.');
}

/** Admin-only registration settings, separate from the public discovery hints. */
export function createRegistrationSettingsAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminServerService, config);
  return {
    async get(options: { signal?: AbortSignal } = {}) {
      const response = await client.getServerConfig({}, options);
      return admission(response.config?.registration?.admission);
    },
    async update(value: RegistrationAdmission) {
      const response = await client.updateServerConfig({
        registration: {
          admission:
            value === 'open' ? AccountCreationPolicy.OPEN : AccountCreationPolicy.INVITE_ONLY
        },
        updateMask: { paths: ['registration.admission'] }
      });
      return admission(response.config?.registration?.admission);
    }
  };
}
