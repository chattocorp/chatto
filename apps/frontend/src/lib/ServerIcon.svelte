<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- fragments and external URLs must bypass SvelteKit resolve */
  import { resolve } from '$app/paths';
  import ServerLogo from './components/ServerLogo.svelte';
  import { NotificationBadge, UnreadDot } from '$lib/ui';
  import type { ServerIndicator } from '$lib/state/server/serverUi';
  import type { Attachment } from 'svelte/attachments';
  import type { ServerStatusBadge } from '$lib/state/server/serverStatus.svelte';
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
    badge = null
  }: {
    /** Display data for the icon (server name + optional logo). */
    server?: { name: string; logoUrl?: string | null };
    /** Icon class name for icon-only mode (e.g., "iconify icon-[uil--comment-alt-lines]") */
    icon?: string;
    href: string;
    selected?: boolean;
    /** What indicator dot (if any) to render below the server. */
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
    /** Optional status marker. The link title describes the cause. */
    badge?: ServerStatusBadge | null;
  } = $props();
</script>

<div class="server-icon-wrapper flex w-12 flex-col items-center gap-1" {@attach contextMenuTrigger}>
  <a
    href={href.startsWith('/') ? resolve(href as '/') : href}
    {onclick}
    {title}
    aria-label={title ?? server?.name}
    class={[
      'server-icon server-gutter-item cursor-pointer',
      selected && 'server-gutter-item-active',
      (badge === 'incompatible' || badge === 'offline') && '[&>div]:opacity-50'
    ]}
    data-testid={server ? 'server-icon' : icon ? 'nav-icon' : undefined}
  >
    {#if server}
      <ServerLogo {server} />
    {:else if icon}
      <span class={icon} aria-hidden="true"></span>
    {/if}
  </a>

  {#if badge || indicator}
    <div class="flex w-full items-center justify-center gap-0.5">
      {#if badge}
        <span
          class={[
            'grid size-4 shrink-0 place-items-center rounded-full',
            badge === 'offline' || badge === 'incompatible'
              ? 'bg-surface-emphasized text-muted'
              : 'bg-warning text-on-warning'
          ]}
          {title}
          data-testid="server-warning"
          aria-hidden="true"
        >
          <span
            class={[
              'iconify text-xs',
              {
                'icon-[uil--key-skeleton]': badge === 'sign-in',
                'icon-[uil--ban]': badge === 'incompatible',
                'icon-[lucide--unplug]': badge === 'offline',
                'icon-[uil--exclamation-circle]': badge === 'compatibility'
              }
            ]}
            aria-hidden="true"
          ></span>
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
            class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
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
                testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
              />
            {/if}
          </button>
        {:else if indicator === 'notification' && notificationCount > 0}
          <NotificationBadge
            count={notificationCount}
            color={importantNotificationCount > 0 ? 'warning' : 'ambient'}
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
            testid={indicator === 'unread' ? 'server-unread-dot' : undefined}
          />
        {/if}
      {/if}
    </div>
  {/if}
</div>
