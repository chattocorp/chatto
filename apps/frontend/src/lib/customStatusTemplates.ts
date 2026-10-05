import {
  CUSTOM_STATUS_TEMPLATES as TEMPLATES,
  getCustomStatusTemplateByToken,
  type CustomStatusTemplate,
  type CustomStatusTemplateId
} from '@chatto/client/util/customStatusTemplates';
import { m } from '$lib/i18n/messages';

/** A custom status template with its translated label. */
export type LabelledCustomStatusTemplate = CustomStatusTemplate & { label: () => string };

const TEMPLATE_LABELS: Record<CustomStatusTemplateId, () => string> = {
  out_for_lunch: () => m('settings.profile.status.template.out_for_lunch'),
  vacation: () => m('settings.profile.status.template.vacation'),
  sick: () => m('settings.profile.status.template.sick')
};

/** The client's templates with translated labels, in display order. */
export const CUSTOM_STATUS_TEMPLATES: LabelledCustomStatusTemplate[] = TEMPLATES.map(
  (template) => ({ ...template, label: TEMPLATE_LABELS[template.id] })
);

/** Shows a template token as its translated label and other status text unchanged. */
export function formatCustomStatusText(text: string): string {
  const template = getCustomStatusTemplateByToken(text);
  return template ? TEMPLATE_LABELS[template.id]() : text;
}
