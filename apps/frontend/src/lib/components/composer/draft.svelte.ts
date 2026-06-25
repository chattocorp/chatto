import { SvelteMap } from 'svelte/reactivity';
import type { FileWithUrl } from './attachments.svelte';

const draftFilesMap = new SvelteMap<string, FileWithUrl[]>();

export function draftKey(roomId: string, threadRootEventId?: string): string {
  return threadRootEventId
    ? `chatto:draft:${roomId}:thread:${threadRootEventId}`
    : `chatto:draft:${roomId}`;
}

export class DraftState {
  key = '';

  switchKey(key: string): string {
    this.key = key;
    return sessionStorage.getItem(key) ?? '';
  }

  persistText(message: string): void {
    if (!this.key) return;
    if (message) {
      sessionStorage.setItem(this.key, message);
    } else {
      sessionStorage.removeItem(this.key);
    }
  }

  clearText(): void {
    if (this.key) sessionStorage.removeItem(this.key);
  }

  takeFiles(): FileWithUrl[] {
    if (!this.key) return [];
    const saved = draftFilesMap.get(this.key) ?? [];
    draftFilesMap.delete(this.key);
    return saved;
  }

  stashFiles(files: FileWithUrl[]): void {
    if (!this.key) return;
    if (files.length > 0) {
      draftFilesMap.set(this.key, files);
    } else {
      draftFilesMap.delete(this.key);
    }
  }

  discardFiles(): void {
    if (this.key) draftFilesMap.delete(this.key);
  }
}
