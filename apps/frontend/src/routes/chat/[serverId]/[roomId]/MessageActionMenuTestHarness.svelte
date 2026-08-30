<script lang="ts">
  import { provideMenuContext } from '$lib/ui/menuContext.svelte';
  import MessageActionMenu from './MessageActionMenu.svelte';
  import type { MessageActionModel } from './messageActionModel';

  let {
    presentation = 'menu',
    action,
    onOpenEmojiPicker,
    onClose
  }: {
    presentation?: 'menu' | 'sheet';
    action: MessageActionModel;
    onOpenEmojiPicker?: () => void;
    onClose: () => void;
  } = $props();

  provideMenuContext({
    presentation: () => (presentation === 'sheet' ? 'sheet' : 'floating'),
    containerRole: () => (presentation === 'sheet' ? 'dialog' : 'menu')
  });
</script>

<MessageActionMenu {presentation} {action} {onOpenEmojiPicker} {onClose} />
