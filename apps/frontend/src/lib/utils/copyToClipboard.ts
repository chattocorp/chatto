import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';

/** Copy text and report success or failure with the standard localized feedback. */
export async function copyToClipboard(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(m('common.copied_to_clipboard'));
  } catch {
    toast.error(m('common.copy_failed'));
  }
}
