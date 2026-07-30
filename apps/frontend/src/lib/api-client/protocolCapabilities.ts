import type { ServerProtocolCapabilities } from '@chatto/api-types/chatto/discovery/v1/server_pb';

export type ProtocolCapabilities = {
  discoveryV1: boolean;
  authV1: boolean;
  apiV1: boolean;
  adminV1: boolean;
  messageSearchV1: boolean;
  roomManagerMemberReadsV1: boolean;
  realtimeV1: boolean;
  realtimeProjectionV1: boolean;
};

const legacyCapabilityKeys = {
  discoveryV1: 'chatto.discovery.v1',
  authV1: 'chatto.auth.v1',
  apiV1: 'chatto.api.v1',
  adminV1: 'chatto.admin.v1',
  messageSearchV1: 'chatto.api.message-search.v1',
  roomManagerMemberReadsV1: 'chatto.api.room-manager-member-reads.v1',
  realtimeV1: 'chatto.realtime.v1',
  realtimeProjectionV1: 'chatto.realtime.projection.v1'
} as const satisfies Record<keyof ProtocolCapabilities, string>;

export type ProtocolCapability = keyof ProtocolCapabilities;

export function protocolCapabilityLabel(capability: ProtocolCapability): string {
  return legacyCapabilityKeys[capability];
}

/**
 * Prefer typed discovery metadata and fall back to the deprecated string list
 * emitted by older servers.
 */
export function mapProtocolCapabilities(
  capabilities: ServerProtocolCapabilities | undefined,
  legacyCapabilities: readonly string[]
): ProtocolCapabilities {
  if (capabilities) {
    return {
      discoveryV1: capabilities.discoveryV1,
      authV1: capabilities.authV1,
      apiV1: capabilities.apiV1,
      adminV1: capabilities.adminV1,
      messageSearchV1: capabilities.messageSearchV1,
      roomManagerMemberReadsV1: capabilities.roomManagerMemberReadsV1,
      realtimeV1: capabilities.realtimeV1,
      realtimeProjectionV1: capabilities.realtimeProjectionV1
    };
  }

  const advertised = new Set(legacyCapabilities);
  return Object.fromEntries(
    Object.entries(legacyCapabilityKeys).map(([capability, key]) => [
      capability,
      advertised.has(key)
    ])
  ) as ProtocolCapabilities;
}
