<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- href is a prop; callers pass already-resolved paths */
  import SpaceLogo from './components/SpaceLogo.svelte';
  import UnreadDot from './ui/UnreadDot.svelte';
  import type { SpaceIndicator } from './state/server/store.svelte';

  let {
    space,
    icon,
    href,
    selected = false,
    indicator = null,
    onIndicatorClick,
    title
  }: {
    /** Display data for the icon (instance name + optional logo). */
    space?: { name: string; logoUrl?: string | null };
    /** Icon class name for icon-only mode (e.g., "iconify uil--comment-alt-lines") */
    icon?: string;
    href: string;
    selected?: boolean;
    /** What indicator dot (if any) to render in the corner. */
    indicator?: SpaceIndicator;
    /** Click handler for the indicator dot. Receives the indicator kind. */
    onIndicatorClick?: (kind: 'notification' | 'unread', event: MouseEvent) => void;
    title?: string;
  } = $props();
</script>

<div class="relative">
  <a
    {href}
    {title}
    aria-label={title ?? space?.name}
    class={['space-icon server-gutter-item cursor-pointer', selected && 'server-gutter-item-active']}
    data-testid={space ? 'space-icon' : icon ? 'nav-icon' : undefined}
  >
    {#if space}
      <SpaceLogo {space} />
    {:else if icon}
      <span class={icon}></span>
    {/if}
  </a>

  {#if indicator}
    {#if onIndicatorClick}
      <button
        type="button"
        onclick={(e) => {
          e.stopPropagation();
          onIndicatorClick(indicator, e);
        }}
        class="absolute -top-1.5 -right-1.5 z-10 flex h-6 w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={indicator === 'notification' ? 'Go to notification' : 'Go to first unread room'}
      >
        <UnreadDot
          color={indicator === 'notification' ? 'warning' : 'muted'}
          overlay
          testid={indicator === 'unread' ? 'space-unread-dot' : undefined}
        />
      </button>
    {:else}
      <UnreadDot
        color={indicator === 'notification' ? 'warning' : 'muted'}
        overlay
        class="absolute top-0 right-0 z-10"
        testid={indicator === 'unread' ? 'space-unread-dot' : undefined}
      />
    {/if}
  {/if}
</div>
