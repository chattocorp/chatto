<!-- @component Shared calendar-day section for activity feeds, including Today. The child snippet receives the section's row-time formatter: elapsed time today, local clock time on older days. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import DaySeparator from './DaySeparator.svelte';
  import {
    formatMessageTime,
    formatRelativeTime,
    type TimeFormatSettings
  } from '$lib/utils/formatTime';

  let {
    dayKey,
    label,
    id,
    testId,
    children
  }: {
    dayKey: string;
    label: string;
    id: string;
    testId?: string;
    children: Snippet<
      [(timestamp: Date | string | null, settings: TimeFormatSettings, locale: string) => string]
    >;
  } = $props();

  /** Use the same day classification as the section heading, including viewer timezone. */
  function rowTime(
    timestamp: Date | string | null,
    settings: TimeFormatSettings,
    locale: string
  ): string {
    if (!timestamp) return '';
    return (dayKey === 'today' ? formatRelativeTime : formatMessageTime)(
      timestamp,
      settings,
      locale
    );
  }
</script>

<section aria-labelledby={id}>
  <DaySeparator {id} {label} {testId} />
  {@render children(rowTime)}
</section>
