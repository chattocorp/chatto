/**
 * Toast notification state and API.
 *
 * Usage:
 *   import { toast } from '$lib/ui/toast';
 *   toast.error("Something went wrong");
 *   toast.success("Message sent");
 *   toast.info("New version available", 0, { label: "Reload", onClick: () => location.reload() });
 */

export type ToastTone = 'error' | 'success' | 'info' | 'warning';

export interface ToastAction {
  label: string;
  onClick: () => void;
}

export interface ToastData {
  id: string;
  tone: ToastTone;
  message: string;
  action?: ToastAction;
}

const DEFAULT_DURATION = 5000;

const toasts = $state<ToastData[]>([]);

function generateId(): string {
  return Math.random().toString(36).substring(2, 9);
}

function add(
  tone: ToastTone,
  message: string,
  duration = DEFAULT_DURATION,
  action?: ToastAction
): string {
  const id = generateId();
  toasts.push({ id, tone, message, action });

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
  error: (message: string, duration?: number, action?: ToastAction) =>
    add('error', message, duration, action),
  success: (message: string, duration?: number, action?: ToastAction) =>
    add('success', message, duration, action),
  info: (message: string, duration?: number, action?: ToastAction) =>
    add('info', message, duration, action),
  warning: (message: string, duration?: number, action?: ToastAction) =>
    add('warning', message, duration, action),
  remove,
  clear
};

export function getToasts(): ToastData[] {
  return toasts;
}
