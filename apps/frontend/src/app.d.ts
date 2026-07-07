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
      modal?: {
        type:
          | 'createRoom'
          | 'logout'
          | 'joinRoom'
          | 'leaveRoom'
          | 'deleteMessage'
          | 'leaveServer'
          | 'deleteAttachment'
          | 'deleteLinkPreview'
          | 'imageViewer';
        spaceId?: string;
        roomId?: string;
        roomName?: string;
        viewerCanJoinRoom?: boolean;
        afterJoinPath?: string;
        closePath?: string;
        spaceName?: string;
        eventId?: string;
        attachmentId?: string;
        attachmentFilename?: string;
        previewUrl?: string;
        imageItems?: Array<{ id?: string; src: string; alt?: string; filename?: string }>;
        imageIndex?: number;
      };
    }
    // interface Platform {}
  }
}

export {};
