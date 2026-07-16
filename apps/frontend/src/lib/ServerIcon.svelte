<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- href is a prop; callers pass already-resolved paths */
  import ServerLogo from './components/ServerLogo.svelte';
  import NotificationBadge from './ui/NotificationBadge.svelte';
  import UnreadDot from './ui/UnreadDot.svelte';
  import type { ServerIndicator } from './state/server/store.svelte';
  import type { Attachment } from 'svelte/attachments';

  let {
    server,
    icon,
    href,
    selected = false,
    indicator = null,
    notificationCount = 0,
    onIndicatorClick,
    contextMenuTrigger,
    title,
    dimmed = false,
    compatibilityWarning = false
  }: {
    /** Display data for the icon (server name + optional logo). */
    server?: { name: string; logoUrl?: string | null };
    /** Icon class name for icon-only mode (e.g., "iconify uil--comment-alt-lines") */
    icon?: string;
    href: string;
    selected?: boolean;
    /** What indicator dot (if any) to render in the corner. */
    indicator?: ServerIndicator;
    /** Number to render for notification indicators. */
    notificationCount?: number;
    /** Click handler for the indicator dot. Receives the indicator kind. */
    onIndicatorClick?: (kind: 'notification' | 'unread', event: MouseEvent) => void;
    /** Optional right-click/long-press behavior for the server link. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    title?: string;
    /** Render as unavailable/degraded while keeping the icon in the gutter. */
    dimmed?: boolean;
    /** Show a non-interactive compatibility warning marker. */
    compatibilityWarning?: boolean;
  } = $props();
</script>

<div class="server-icon-wrapper relative" {@attach contextMenuTrigger}>
  <a
    {href}
    {title}
    aria-label={title ?? server?.name}
    class={[
      'server-icon server-gutter-item cursor-pointer',
      selected && 'server-gutter-item-active',
      dimmed && 'opacity-40 grayscale'
    ]}
    data-testid={server ? 'server-icon' : icon ? 'nav-icon' : undefined}
  >
    {#if server}
      <ServerLogo {server} />
    {:else if icon}
      <span class={icon}></span>
    {/if}
  </a>

  {#if compatibilityWarning}
    <span
      class="pointer-events-none absolute -top-1 -left-1 z-10 flex h-5 w-5 items-center justify-center rounded-full bg-warning text-on-warning shadow-sm"
      data-testid="server-compatibility-warning"
      aria-hidden="true"
    >
      <span class="iconify text-xs uil--exclamation-circle"></span>
    </span>
  {/if}

  {#if indicator}
    {#if onIndicatorClick}
      <button
        type="button"
        onclick={(e) => {
          e.stopPropagation();
          onIndicatorClick(indicator, e);
        }}
        class="absolute -top-1.5 -right-1.5 z-10 flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={indicator === 'notification' && notificationCount > 0
          ? `Go to ${notificationCount} notifications`
          : indicator === 'notification'
            ? 'Go to notification'
            : 'Go to first unread room'}
      >
        {#if indicator === 'notification' && notificationCount > 0}
          <NotificationBadge
            count={notificationCount}
            overlay
            testid="server-notification-badge"
          />
          <span class="sr-only">{notificationCount} notifications</span>
        {:else}
          <UnreadDot
            color={indicator === 'notification' ? 'warning' : 'muted'}
            overlay
            testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
          />
        {/if}
      </button>
    {:else}
      {#if indicator === 'notification' && notificationCount > 0}
        <NotificationBadge
          count={notificationCount}
          overlay
          class="absolute top-0 right-0 z-10"
          testid="server-notification-badge"
        />
        <span class="sr-only">{notificationCount} notifications</span>
      {:else}
        <UnreadDot
          color={indicator === 'notification' ? 'warning' : 'muted'}
          overlay
          class="absolute top-0 right-0 z-10"
          testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
        />
      {/if}
    {/if}
  {/if}
</div>
