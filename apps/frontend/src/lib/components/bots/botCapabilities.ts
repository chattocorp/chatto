/**
 * Bot capabilities: presets of bot permission grants that the Create Bot
 * dialog offers. A capability is a frontend concept only. The server stores
 * the resulting direct allows on the bot and knows nothing about the preset.
 * See FDR-038.
 */

import type { PermissionAPI, PermissionScope } from '@chatto/client/api/permissions';
import { errorMessage } from '$lib/utils/errorMessage';

/** One allow that a capability writes on the new bot. */
export type BotCapabilityGrant = {
  scope: Extract<PermissionScope, { tier: 'server' } | { tier: 'dm' }>;
  permission: string;
};

/** A combinable preset of bot grants, shown as one tile. */
export type BotCapability = {
  /** Stable key, also used for the i18n keys `settings.bots.capabilities.<id>`. */
  id: BotCapabilityId;
  /** Iconify class of the tile icon. */
  icon: string;
  /** The allows that the capability writes. All of them must succeed for the capability to work. */
  grants: readonly BotCapabilityGrant[];
};

export type BotCapabilityId =
  'join_rooms' | 'interactions' | 'read_all' | 'post' | 'direct_messages' | 'react' | 'attach';

const SERVER = { tier: 'server' } as const;
const DM = { tier: 'dm' } as const;

/**
 * The capabilities in display order. A server-scope allow also applies in the
 * DMs that the bot is in, because a DM check falls back to server scope.
 */
export const BOT_CAPABILITIES: readonly BotCapability[] = [
  {
    id: 'interactions',
    icon: 'icon-[uil--at]',
    grants: [
      { scope: SERVER, permission: 'message.read-interactions' },
      { scope: SERVER, permission: 'message.post-in-interactions' }
    ]
  },
  {
    id: 'direct_messages',
    icon: 'icon-[uil--envelope]',
    grants: [
      { scope: DM, permission: 'message.read' },
      { scope: DM, permission: 'message.post' }
    ]
  },
  {
    id: 'post',
    icon: 'icon-[uil--comment-alt-message]',
    grants: [
      { scope: SERVER, permission: 'message.post' },
      { scope: SERVER, permission: 'message.echo' }
    ]
  },
  {
    id: 'read_all',
    icon: 'icon-[uil--eye]',
    grants: [{ scope: SERVER, permission: 'message.read' }]
  },
  {
    id: 'react',
    icon: 'icon-[uil--smile]',
    grants: [{ scope: SERVER, permission: 'message.react' }]
  },
  {
    id: 'attach',
    icon: 'icon-[uil--paperclip]',
    grants: [{ scope: SERVER, permission: 'message.attach' }]
  },
  {
    id: 'join_rooms',
    icon: 'icon-[uil--compass]',
    grants: [
      { scope: SERVER, permission: 'room.list' },
      { scope: SERVER, permission: 'room.join' }
    ]
  }
];

const CAPABILITIES_BY_ID = new Map(
  BOT_CAPABILITIES.map((capability) => [capability.id, capability])
);

/** A grant together with the capability that requested it first. */
export type BotCapabilityGrantRequest = BotCapabilityGrant & { capabilityId: BotCapabilityId };

/**
 * Returns the grants of the selected capabilities in catalogue order. A grant
 * that two capabilities share appears once.
 */
export function botCapabilityGrants(ids: Iterable<BotCapabilityId>): BotCapabilityGrantRequest[] {
  const selected = new Set(ids);
  const seen = new Set<string>();
  const grants: BotCapabilityGrantRequest[] = [];
  for (const capability of BOT_CAPABILITIES) {
    if (!selected.has(capability.id)) continue;
    for (const grant of capability.grants) {
      const key = `${grant.scope.tier}:${grant.permission}`;
      if (seen.has(key)) continue;
      seen.add(key);
      grants.push({ ...grant, capabilityId: capability.id });
    }
  }
  return grants;
}

/**
 * Reports whether the creator can give the capability to a bot they own. The
 * server rejects a bot grant that the owner does not hold (owner ceiling), so
 * the dialog locks such a capability. It checks every grant against the
 * creator's server-scope permissions. A DM-scope decision of the creator can
 * differ; the apply step reports that case.
 */
export function botCapabilityAvailable(
  id: BotCapabilityId,
  serverScope: Readonly<Record<string, boolean>>
): boolean {
  const capability = CAPABILITIES_BY_ID.get(id);
  return !!capability && capability.grants.every((grant) => serverScope[grant.permission] === true);
}

/** A grant that the server did not accept. */
export type BotCapabilityFailure = BotCapabilityGrantRequest & { error: string };

/**
 * Writes the grants of the selected capabilities on the bot, one request at a
 * time. It continues after a failed grant and returns all failures; it does
 * not throw for a rejected grant.
 */
export async function applyBotCapabilities(
  api: PermissionAPI,
  botId: string,
  ids: Iterable<BotCapabilityId>
): Promise<{ failed: BotCapabilityFailure[] }> {
  const failed: BotCapabilityFailure[] = [];
  for (const grant of botCapabilityGrants(ids)) {
    try {
      await api.setUserPermission({
        userId: botId,
        scope: grant.scope,
        permission: grant.permission,
        state: 'allow'
      });
    } catch (error) {
      failed.push({ ...grant, error: errorMessage(error) });
    }
  }
  return { failed };
}
