import { createContext } from 'svelte';
import type { Bot } from '$lib/api-client/bots';

/**
 * The bot that the bot detail layout loads, shared with its section pages.
 *
 * The layout owns the bot query. SvelteKit reuses the layout and its section
 * pages when only the bot ID changes, so every mutation must capture `botId`
 * before it starts and check `isCurrentTarget` before it applies its result.
 */
export interface BotDetailContext {
  /** Bot ID from the current route. */
  readonly botId: string;
  /** The loaded bot, or `null` while the read is pending or failed. */
  readonly bot: Bot | null;
  /** True until the first bot read completes. */
  readonly isPending: boolean;
  /** Owners and bot managers can manage credentials, webhooks, and permissions. */
  readonly canOperateBot: boolean;
  /** Owners, bot managers, and account managers can edit the public identity. */
  readonly canEditIdentity: boolean;
  /**
   * True while the layout is mounted, the server scope is current, and the
   * route still shows the bot that `mutationTarget` names.
   */
  isCurrentTarget(mutationTarget: string): boolean;
  /** Writes a returned bot into the query cache and refreshes bot lists. */
  cacheBot(updated: Bot): void;
  /** Refetches the bot and bot lists after a mutation that returns no bot. */
  refreshBot(): void;
}

const [getBotDetail, setBotDetail] = createContext<BotDetailContext>();

/** Provides the bot detail context to the section pages. */
export function provideBotDetail(context: BotDetailContext): void {
  setBotDetail(context);
}

/** Returns the bot detail context provided by the bot detail layout. */
export function useBotDetail(): BotDetailContext {
  return getBotDetail();
}
