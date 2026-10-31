/** Optional room context shared by Markdown fields and their rendered content. */
import type { RoomMember } from '$lib/state/room';
import type { RoomMembersStore } from '@chatto/client/room/members';
import type { MentionRole } from './autocomplete.svelte';

export type MarkdownMentionOptions = {
  members: RoomMember[];
  /** Lookup owner for mentions in rendered content. */
  memberStore?: RoomMembersStore;
  roles?: MentionRole[];
  viewerLogin?: string;
  /** Uses the existing member directory to find users beyond loaded pages. */
  searchMembers?: (query: string) => Promise<RoomMember[]>;
  onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
};
