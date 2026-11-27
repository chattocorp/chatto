/**
 * Device-local state of Server Overview cards: hidden or collapsed cards.
 * Only a flag per server is stored, never the card content.
 */

import { Codecs, serverSlot } from '@chatto/client/storage/slot';

/** An Overview card that the viewer can hide on this device. */
export type OverviewCard = 'setup';

function slot(serverId: string, card: OverviewCard) {
  return serverSlot(serverId, `overview:${card}Dismissed`, false, Codecs.boolean);
}

/** Whether the viewer hid this card for this server on this device. */
export function isOverviewCardDismissed(serverId: string, card: OverviewCard): boolean {
  return slot(serverId, card).get();
}

/** Hide this card for this server on this device. */
export function dismissOverviewCard(serverId: string, card: OverviewCard): void {
  slot(serverId, card).set(true);
}

function welcomeCollapsedSlot(serverId: string) {
  return serverSlot(serverId, 'overview:welcomeCollapsed', false, Codecs.boolean);
}

/** Whether the viewer collapsed the welcome message for this server on this device. */
export function isWelcomeCollapsed(serverId: string): boolean {
  return welcomeCollapsedSlot(serverId).get();
}

/** Remember whether the welcome message is collapsed for this server on this device. */
export function setWelcomeCollapsed(serverId: string, collapsed: boolean): void {
  welcomeCollapsedSlot(serverId).set(collapsed);
}
