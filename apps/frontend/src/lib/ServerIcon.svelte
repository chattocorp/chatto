<script lang="ts">
  import ServerLogo from './components/ServerLogo.svelte';
  import NotificationBadge from './ui/NotificationBadge.svelte';
  import UnreadDot from './ui/UnreadDot.svelte';
  import type { ServerIndicator } from './state/server/store.svelte';

  let {
    server,
    icon,
    href,
    selected = false,
    indicator = null,
    notificationCount = 0,
    onIndicatorClick,
    title,
    dimmed = false,
    home = false,
    homeLabel
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
    title?: string;
    /** Render as unavailable/degraded while keeping the icon in the gutter. */
    dimmed?: boolean;
    /** Mark this server as the user's client-sync home server. */
    home?: boolean;
    /** Accessible label for the home-server marker. */
    homeLabel?: string;
  } = $props();
</script>

<div class="server-icon-wrapper relative">
  <!-- eslint-disable svelte/no-navigation-without-resolve -- callers provide resolved internal routes or absolute server URLs -->
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
  <!-- eslint-enable svelte/no-navigation-without-resolve -->

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
          <NotificationBadge count={notificationCount} overlay testid="server-notification-badge" />
          <span class="sr-only">{notificationCount} notifications</span>
        {:else}
          <UnreadDot
            color={indicator === 'notification' ? 'warning' : 'muted'}
            overlay
            testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
          />
        {/if}
      </button>
    {:else if indicator === 'notification' && notificationCount > 0}
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

  {#if home}
    <span
      class="absolute bottom-0 left-0 z-10 flex size-5 items-center justify-center rounded-full bg-action text-on-action shadow-sm ring-2 ring-background"
      role={homeLabel ? 'img' : 'presentation'}
      aria-label={homeLabel}
      data-testid="home-server-indicator"
    >
      <svg class="size-3" viewBox="0 0 20 20" aria-hidden="true">
        <path fill="currentColor" d="M10 2.5 2.5 8.7v8.8h5v-5h5v5h5V8.7L10 2.5Z" />
      </svg>
    </span>
  {/if}
</div>
