import { Codecs, globalSlot } from './slot';

export const MAIN_SIDEBAR_DEFAULT_OPEN = true;

const slot = globalSlot('mainSidebarOpen', MAIN_SIDEBAR_DEFAULT_OPEN, Codecs.boolean);

export function getMainSidebarOpen(): boolean {
  return slot.get();
}

export function setMainSidebarOpen(open: boolean): void {
  slot.set(open);
}
