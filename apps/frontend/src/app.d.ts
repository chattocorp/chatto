import type { ChatModal, FrameViewModal } from '$lib/modal';

// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
  namespace App {
    // interface Error {}
    // interface Locals {}
    // interface PageData {}
    interface PageState {
      threadFilter?: 'all' | 'unread';
      welcome?: boolean;
      modal?: ChatModal;
      /** A frame view that stays open below the dialog in `modal`. */
      frameView?: FrameViewModal;
    }
    // interface Platform {}
  }
}

export {};
