<script lang="ts">
  import { provideMenuContext } from '$lib/ui/menuContext.svelte';
  import MessageActionMenu from './MessageActionMenu.svelte';
  import type { MessageActionModel } from './messageActionModel';

  let {
    presentation = 'menu',
    action,
    linkUrl = null,
    imageUrl = null,
    onOpenEmojiPicker,
    hasReactions = false,
    onOpenReactionDetails,
    onClose
  }: {
    presentation?: 'menu' | 'sheet';
    action: MessageActionModel;
    linkUrl?: string | null;
    imageUrl?: string | null;
    onOpenEmojiPicker?: () => void;
    hasReactions?: boolean;
    onOpenReactionDetails?: () => void;
    onClose: () => void;
  } = $props();

  provideMenuContext({
    presentation: () => (presentation === 'sheet' ? 'sheet' : 'floating'),
    containerRole: () => (presentation === 'sheet' ? 'dialog' : 'menu')
  });
</script>

<MessageActionMenu
  {presentation}
  {action}
  {linkUrl}
  {imageUrl}
  {onOpenEmojiPicker}
  {hasReactions}
  {onOpenReactionDetails}
  {onClose}
/>
