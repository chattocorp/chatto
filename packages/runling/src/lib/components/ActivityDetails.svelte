<script lang="ts">
  import { activityStatus, type Activity } from '$lib/timeline.ts';
  import { duration } from '$lib/runs.ts';
  import AnsiText from './AnsiText.svelte';
  import Usage from './Usage.svelte';
  import RunValue from './RunValue.svelte';

  let {
    activity,
    elapsed,
    onclose,
    titleId
  }: {
    activity: Activity;
    elapsed: number;
    onclose: () => void;
    titleId: string;
  } = $props();
</script>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <header
    class="flex shrink-0 items-start justify-between gap-4 border-b border-base-300 px-5 py-4"
  >
    <div class="min-w-0 space-y-2">
      <h2 id={titleId} class="text-lg font-medium wrap-anywhere">{activity.label}</h2>
      <p class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-base-content/60">
        <span>{activity.kind}</span>
        <span>{activityStatus(activity)}</span>
        <span>Started at {duration(activity.startedAt)}</span>
        <span
          >{activity.kind === 'input' ? 'Wait' : 'Duration'}
          {duration(activity.durationMs ?? Math.max(0, elapsed - activity.startedAt))}</span
        >
      </p>
    </div>
    <button
      class="btn btn-ghost btn-sm btn-square shrink-0"
      onclick={onclose}
      aria-label="Close activity details"
    >
      <span class="icon-[lucide--x] size-4" aria-hidden="true"></span>
    </button>
  </header>
  <div class="min-h-0 space-y-5 overflow-auto px-5 py-5">
    {#if activity.usage}<Usage usage={activity.usage} detail />{/if}
    {#if activity.state !== undefined}
      <div>
        <RunValue value={activity.state} kind="state" />
        <p class="mt-2 text-xs text-base-content/60">
          Latest snapshot at {duration(activity.stateAt ?? 0)}
        </p>
      </div>
    {/if}
    <div>
      <h3 class="mb-3 text-sm font-medium">Activity logs</h3>
      <pre class="m-0 whitespace-pre-wrap font-mono text-xs leading-relaxed wrap-anywhere"><AnsiText
          text={activity.logs.length ? activity.logs.join('\n\n') : 'No logs for this activity.'}
        /></pre>
    </div>
  </div>
</div>
