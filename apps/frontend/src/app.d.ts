import type { ChatModal } from '$lib/modal';

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
    }
    // interface Platform {}
  }
}

export {};
