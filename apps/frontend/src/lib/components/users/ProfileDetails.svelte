<!--
@component

Profile facts below the bio: the user's current local time and its difference
from the viewer's time zone, the account creation date, and, unless the host
shows them elsewhere, the assigned roles. Local time needs a shared time zone.
Account age and roles come from the shared profile-details read; rows stay
hidden until their data is available.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    formatDate,
    formatHourDifference,
    formatMessageTime,
    timeFormatSettingsFor,
    timeZoneDifferenceMinutes,
    type ViewerTimeSettings
  } from '$lib/utils/formatTime';
  import { useProfileDetails } from './profileDetailsQuery.svelte';
  import ProfileRoles from './ProfileRoles.svelte';

  let {
    userId,
    timezone,
    viewerSettings,
    showRoles = true,
    class: className
  }: {
    userId: string;
    /** Live shared IANA time zone, or null when the user does not share one. */
    timezone: string | null;
    viewerSettings?: ViewerTimeSettings | null;
    /** False when the host shows the roles in a section of their own. */
    showRoles?: boolean;
    class?: string;
  } = $props();

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

  const hasRoles = $derived(showRoles && !!details.data?.roles.length);
</script>

{#if localTime || memberSince || hasRoles}
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
    {#if hasRoles}
      <ProfileRoles {userId} />
    {/if}
  </div>
{/if}
