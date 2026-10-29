<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, within } from 'storybook/test';
  import ActivityDaySection from './ActivityDaySection.svelte';

  const { Story } = defineMeta({
    title: 'Components/Activity day section',
    component: ActivityDaySection,
    tags: ['autodocs']
  });
</script>

<Story
  name="Day headings and times"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('heading', { name: 'Today' })).toBeVisible();
    await expect(canvas.getByTestId('today-time')).toHaveTextContent('Just now');
    await expect(canvas.getByTestId('older-time')).toHaveTextContent('14:30');
    await expect(canvas.getByTestId('timezone-time')).toHaveTextContent('10:30 AM');
    await expect(canvas.getByTestId('unknown-time').textContent).toBe('');
  }}
>
  <ActivityDaySection dayKey="today" label="Today" id="activity-today">
    {#snippet children(rowTime)}
      <span data-testid="today-time"
        >{rowTime(new Date(), { effectiveTimezone: 'UTC', effectiveHour12: false }, 'en-GB')}</span
      >
    {/snippet}
  </ActivityDaySection>
  <ActivityDaySection dayKey="2026-10-12" label="12 Oct 2026" id="activity-older">
    {#snippet children(rowTime)}
      <span data-testid="older-time"
        >{rowTime(
          '2026-10-12T14:30:00Z',
          { effectiveTimezone: 'UTC', effectiveHour12: false },
          'en-GB'
        )}</span
      >
      <span data-testid="timezone-time"
        >{rowTime(
          '2026-10-12T14:30:00Z',
          { effectiveTimezone: 'America/New_York', effectiveHour12: true },
          'en-US'
        )}</span
      >
    {/snippet}
  </ActivityDaySection>
  <ActivityDaySection dayKey="unknown" label="Unknown" id="activity-unknown">
    {#snippet children(rowTime)}
      <span data-testid="unknown-time"
        >{rowTime(null, { effectiveTimezone: 'UTC', effectiveHour12: false }, 'en-GB')}</span
      >
    {/snippet}
  </ActivityDaySection>
</Story>
