<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- fragments and external URLs must bypass SvelteKit resolve */
  import { resolve } from '$app/paths';
  import ServerLogo from './components/ServerLogo.svelte';
  import NotificationBadge from './ui/NotificationBadge.svelte';
  import UnreadDot from './ui/UnreadDot.svelte';
  import type { ServerIndicator } from './state/server/store.svelte';
  import type { Attachment } from 'svelte/attachments';
  import { m } from '$lib/i18n/messages';

  let {
    server,
    icon,
    href,
    selected = false,
    indicator = null,
    notificationCount = 0,
    importantNotificationCount = 0,
    onclick,
    onIndicatorClick,
    contextMenuTrigger,
    title,
    dimmed = false,
    signInRequired = false,
    compatibilityWarning = false
  }: {
    /** Display data for the icon (server name + optional logo). */
    server?: { name: string; logoUrl?: string | null };
    /** Icon class name for icon-only mode (e.g., "iconify icon-[uil--comment-alt-lines]") */
    icon?: string;
    href: string;
    selected?: boolean;
    /** What indicator dot (if any) to render in the corner. */
    indicator?: ServerIndicator;
    /** Number to render for notification indicators. */
    notificationCount?: number;
    /** Number of unread notifications that should use notification orange. */
    importantNotificationCount?: number;
    /** Optional click behavior for the server link. */
    onclick?: (event: MouseEvent) => void;
    /** Click handler for the indicator dot. Receives the indicator kind. */
    onIndicatorClick?: (kind: 'notification' | 'unread', event: MouseEvent) => void;
    /** Optional right-click/long-press behavior for the server link. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    title?: string;
    /** Render as unavailable/degraded while keeping the icon in the gutter. */
    dimmed?: boolean;
    /** Show that this server requires the user to sign in. */
    signInRequired?: boolean;
    /** Show a non-interactive compatibility warning marker. */
    compatibilityWarning?: boolean;
  } = $props();
</script>

<div class="server-icon-wrapper relative" {@attach contextMenuTrigger}>
  <a
    href={href.startsWith('/') ? resolve(href as '/') : href}
    {onclick}
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
      <span class={icon} aria-hidden="true"></span>
    {/if}
  </a>

  {#if signInRequired}
    <span
      class="pointer-events-none absolute -start-1 -top-1 z-10 flex h-5 w-5 items-center justify-center rounded-full bg-warning text-on-warning"
      data-testid="server-sign-in-required"
      aria-hidden="true"
    >
      <span class="iconify icon-[uil--exclamation-circle] text-xs" aria-hidden="true"></span>
    </span>
  {:else if compatibilityWarning}
    <span
      class="pointer-events-none absolute -start-1 -top-1 z-10 flex h-5 w-5 items-center justify-center rounded-full bg-warning text-on-warning"
      data-testid="server-compatibility-warning"
      aria-hidden="true"
    >
      <span class="iconify icon-[uil--exclamation-circle] text-xs" aria-hidden="true"></span>
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
        class="absolute -end-1.5 -top-1.5 z-10 flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={indicator === 'notification' && notificationCount > 0
          ? m('chat.server_gutter.go_to_notifications_count', { count: notificationCount })
          : indicator === 'notification'
            ? m('chat.server_gutter.go_to_notification')
            : m('chat.server_gutter.go_to_first_unread')}
      >
        {#if indicator === 'notification' && notificationCount > 0}
          <NotificationBadge
            count={notificationCount}
            color={importantNotificationCount > 0 ? 'warning' : 'ambient'}
            overlay
            testid="server-notification-badge"
          />
          <span class="sr-only"
            >{m('chat.server_gutter.notifications_count', { count: notificationCount })}</span
          >
        {:else}
          <UnreadDot
            color={indicator === 'notification'
              ? importantNotificationCount > 0
                ? 'warning'
                : 'ambient'
              : 'muted'}
            overlay
            testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
          />
        {/if}
      </button>
    {:else if indicator === 'notification' && notificationCount > 0}
      <NotificationBadge
        count={notificationCount}
        color={importantNotificationCount > 0 ? 'warning' : 'ambient'}
        overlay
        class="absolute end-0 top-0 z-10"
        testid="server-notification-badge"
      />
      <span class="sr-only"
        >{m('chat.server_gutter.notifications_count', { count: notificationCount })}</span
      >
    {:else}
      <UnreadDot
        color={indicator === 'notification'
          ? importantNotificationCount > 0
            ? 'warning'
            : 'ambient'
          : 'muted'}
        overlay
        class="absolute end-0 top-0 z-10"
        testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
      />
    {/if}
  {/if}
</div>
