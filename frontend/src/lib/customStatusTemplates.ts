import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';
import * as m from '$lib/i18n/messages';

export const CUSTOM_STATUS_TEMPLATE_PREFIX = 'chatto:status:';

export type CustomStatusTemplateId = 'out_for_lunch' | 'vacation' | 'sick';

export type CustomStatusTemplate = {
  id: CustomStatusTemplateId;
  emoji: string;
  token: string;
  defaultExpiryMinutes?: number;
  label: () => string;
};

export const CUSTOM_STATUS_TEMPLATES: CustomStatusTemplate[] = [
  {
    id: 'out_for_lunch',
    emoji: '🍽️',
    token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}out_for_lunch`,
    defaultExpiryMinutes: 60,
    label: () => m['settings.profile.status.template.out_for_lunch']()
  },
  {
    id: 'vacation',
    emoji: '🌴',
    token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}vacation`,
    label: () => m['settings.profile.status.template.vacation']()
  },
  {
    id: 'sick',
    emoji: '🤒',
    token: `${CUSTOM_STATUS_TEMPLATE_PREFIX}sick`,
    label: () => m['settings.profile.status.template.sick']()
  }
];

export function getCustomStatusTemplateById(
  id: CustomStatusTemplateId
): CustomStatusTemplate | undefined {
  return CUSTOM_STATUS_TEMPLATES.find((template) => template.id === id);
}

export function getCustomStatusTemplateByToken(token: string): CustomStatusTemplate | undefined {
  return CUSTOM_STATUS_TEMPLATES.find((template) => template.token === token);
}

export function getCustomStatusTemplate(status: CustomUserStatus | null | undefined) {
  if (!status) return undefined;
  const template = getCustomStatusTemplateByToken(status.text);
  return template?.emoji === status.emoji ? template : undefined;
}

export function formatCustomStatusText(text: string): string {
  return getCustomStatusTemplateByToken(text)?.label() ?? text;
}

export function customStatusTemplateText(id: CustomStatusTemplateId): string {
  return getCustomStatusTemplateById(id)?.token ?? '';
}

export function defaultTemplateExpiry(id: CustomStatusTemplateId): Date | null {
  const minutes = getCustomStatusTemplateById(id)?.defaultExpiryMinutes;
  return minutes ? new Date(Date.now() + minutes * 60_000) : null;
}
