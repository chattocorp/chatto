/**
 * Toast notification state and API.
 *
 * Usage:
 *   import { toast } from '$lib/ui/toast';
 *   toast.error("Something went wrong");
 *   toast.success("Message sent");
 *   toast.info("New version available", 0, { label: "Reload", onClick: () => location.reload() });
 */
import type { AccountNameIdentity } from '$lib/render/accountName';

export type ToastTone = 'error' | 'success' | 'info' | 'warning';

export interface ToastAction {
  label: string;
  onClick: () => void;
}

export interface ToastData {
  id: string;
  tone: ToastTone;
  message: string;
  accounts?: readonly { name: string; identity?: AccountNameIdentity | null }[];
  action?: ToastAction;
}

/** A localized sentence can carry account identities for visual bot badges. */
export type ToastMessage =
  | string
  | {
      text: string;
      accounts: readonly { name: string; identity?: AccountNameIdentity | null }[];
    };

const DEFAULT_DURATION = 5000;

const toasts = $state<ToastData[]>([]);

function generateId(): string {
  return Math.random().toString(36).substring(2, 9);
}

function add(
  tone: ToastTone,
  message: ToastMessage,
  duration = DEFAULT_DURATION,
  action?: ToastAction
): string {
  const id = generateId();
  toasts.push(
    typeof message === 'string'
      ? { id, tone, message, action }
      : { id, tone, message: message.text, accounts: message.accounts, action }
  );

  if (duration > 0) {
    setTimeout(() => remove(id), duration);
  }

  return id;
}

function remove(id: string): void {
  const index = toasts.findIndex((t) => t.id === id);
  if (index !== -1) {
    toasts.splice(index, 1);
  }
}

function clear(): void {
  toasts.length = 0;
}

export const toast = {
  error: (message: ToastMessage, duration?: number, action?: ToastAction) =>
    add('error', message, duration, action),
  success: (message: ToastMessage, duration?: number, action?: ToastAction) =>
    add('success', message, duration, action),
  info: (message: ToastMessage, duration?: number, action?: ToastAction) =>
    add('info', message, duration, action),
  warning: (message: ToastMessage, duration?: number, action?: ToastAction) =>
    add('warning', message, duration, action),
  remove,
  clear
};

export function getToasts(): ToastData[] {
  return toasts;
}
