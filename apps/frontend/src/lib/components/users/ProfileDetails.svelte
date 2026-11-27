<!--
@component

Profile facts below the bio: the user's current local time and its difference
from the viewer's time zone, the account creation date, and assigned roles.
Local time needs a shared time zone. Account age and roles come from the shared
profile-details read; rows stay hidden until their data is available.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    formatDate,
    formatHourDifference,
    formatMessageTime,
    timeFormatSettingsFor,
    timeZoneDifferenceMinutes,
    type ViewerTimeSettings
  } from '$lib/utils/formatTime';
  import { useProfileDetails } from './profileDetailsQuery.svelte';

  let {
    userId,
    timezone,
    viewerSettings,
    class: className
  }: {
    userId: string;
    /** Live shared IANA time zone, or null when the user does not share one. */
    timezone: string | null;
    viewerSettings?: ViewerTimeSettings | null;
    class?: string;
  } = $props();

  const serverScope = useServerScope();
  const details = useProfileDetails(() => userId);
  const timeSettings = $derived(timeFormatSettingsFor(viewerSettings));
  const locale = $derived(getLocale());
  // Re-render the local-time row once a minute while it is shown.
  let now = $state(Date.now());

  const localTime = $derived.by(() => {
    if (!timezone) return null;
    try {
      return formatMessageTime(new Date(now), { ...timeSettings, effectiveTimezone: timezone });
    } catch {
      return null;
    }
  });

  const offsetLabel = $derived.by(() => {
    if (!timezone) return null;
    const minutes = timeZoneDifferenceMinutes(timezone, timeSettings, new Date(now));
    if (minutes === null) return null;
    if (minutes === 0) return m('chat.profile.time_offset.same');
    const offset = formatHourDifference(minutes, locale);
    return minutes > 0
      ? m('chat.profile.time_offset.ahead', { offset })
      : m('chat.profile.time_offset.behind', { offset });
  });

  const memberSince = $derived.by(() => {
    const createdAt = details.data?.createdAt;
    return createdAt ? formatDate(createdAt, timeSettings, locale) : null;
  });

  /** Assigned roles in catalogue order; roles missing from the catalogue keep their name. */
  const roles = $derived.by(() => {
    const assigned = details.data?.roles ?? [];
    if (!assigned.length) return [];
    const catalogue = serverScope.store.mentionRoles.roles;
    const position = new Map(catalogue.map((role, index) => [role.name, index]));
    const label = new Map(catalogue.map((role) => [role.name, role.displayName || role.name]));
    return [...assigned]
      .sort((a, b) => (position.get(a) ?? Infinity) - (position.get(b) ?? Infinity))
      .map((name) => ({ name, label: label.get(name) ?? name }));
  });
</script>

{#if localTime || memberSince || roles.length}
  <div class={['flex flex-col gap-2 text-start', className]} data-testid="profile-details">
    {#if localTime}
      <div class="flex items-start gap-2" data-testid="profile-local-time">
        <span class="iconify mt-0.5 icon-[uil--clock-three] shrink-0 text-muted" aria-hidden="true"
        ></span>
        <div class="min-w-0">
          <div>{m('chat.profile.local_time', { time: localTime })}</div>
          <div class="truncate text-muted">
            <span dir="ltr">{timezone}</span>{#if offsetLabel}
              · {offsetLabel}{/if}
          </div>
        </div>
      </div>
      <Interval milliseconds={60_000} ontick={() => (now = Date.now())} />
    {/if}
    {#if memberSince}
      <div class="flex items-center gap-2" data-testid="profile-member-since">
        <span class="iconify icon-[uil--calendar-alt] shrink-0 text-muted" aria-hidden="true"
        ></span>
        <span class="min-w-0">{m('chat.profile.member_since', { date: memberSince })}</span>
      </div>
    {/if}
    {#if roles.length}
      <ul
        class="flex flex-wrap gap-1.5"
        aria-label={m('chat.profile.roles')}
        data-testid="profile-roles"
      >
        {#each roles as role (role.name)}
          <li class="max-w-full truncate rounded-full bg-surface px-2 py-0.5">
            <bdi>{role.label}</bdi>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
