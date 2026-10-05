import type { CustomUserStatus } from '../api/userSummary.js';

/*
 * Custom status templates. A template status stores a reserved token, such as
 * `chatto:status:vacation`, as its status text, together with the template's
 * emoji. Each host shows the token as translated text, so every reader sees
 * the template in their own language. A host that does not know a token shows
 * the stored text.
 */

/** Prefix of every template token in the status text. */
export const CUSTOM_STATUS_TEMPLATE_PREFIX = 'chatto:status:';

/** IDs of the known templates. */
export type CustomStatusTemplateId = 'out_for_lunch' | 'vacation' | 'sick';

/** One template without its display text. Hosts supply the label for each `id`. */
export type CustomStatusTemplate = {
  id: CustomStatusTemplateId;
  emoji: string;
  /** Status text that identifies this template. */
  token: string;
  /** Expiry to suggest when a person selects the template. */
  defaultExpiryMinutes?: number;
};

/** All known templates, in display order. */
export const CUSTOM_STATUS_TEMPLATES: readonly CustomStatusTemplate[] = [
  {
    id: 'out_for_lunch',
    emoji: '🍽️',
    token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}out_for_lunch`,
    defaultExpiryMinutes: 60
  },
  { id: 'vacation', emoji: '🌴', token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}vacation` },
  { id: 'sick', emoji: '🤒', token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}sick` }
];

/** Returns the template with this ID. */
export function getCustomStatusTemplateById(
  id: CustomStatusTemplateId
): CustomStatusTemplate | undefined {
  return CUSTOM_STATUS_TEMPLATES.find((template) => template.id === id);
}

/** Returns the template whose token is exactly `text`. */
export function getCustomStatusTemplateByToken(text: string): CustomStatusTemplate | undefined {
  return CUSTOM_STATUS_TEMPLATES.find((template) => template.token === text);
}

/**
 * Returns the template of a status. A status is a template only when both its
 * text and its emoji match; a changed emoji makes it a custom status.
 */
export function getCustomStatusTemplate(
  status: CustomUserStatus | null | undefined
): CustomStatusTemplate | undefined {
  if (!status) return undefined;
  const template = getCustomStatusTemplateByToken(status.text);
  return template?.emoji === status.emoji ? template : undefined;
}

/** Status text to store for a template. */
export function customStatusTemplateText(id: CustomStatusTemplateId): string {
  return getCustomStatusTemplateById(id)?.token ?? '';
}

/** Suggested expiry for a template, counted from now, or null when it has none. */
export function defaultTemplateExpiry(id: CustomStatusTemplateId): Date | null {
  const minutes = getCustomStatusTemplateById(id)?.defaultExpiryMinutes;
  return minutes ? new Date(Date.now() + minutes * 60_000) : null;
}
