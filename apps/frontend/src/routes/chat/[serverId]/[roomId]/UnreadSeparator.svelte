<script lang="ts">
  import * as m from '$lib/i18n/messages';
  // UnreadSeparator - shows a visual line between read and unread messages
</script>

<div class="mt-4 flex items-center gap-4 py-3">
  <div class="h-px flex-1 bg-warning/50"></div>
  <span class="text-xs font-medium text-warning/70">{m['room.unread_separator']()}</span>
  <div class="h-px flex-1 bg-warning/50"></div>
</div>
