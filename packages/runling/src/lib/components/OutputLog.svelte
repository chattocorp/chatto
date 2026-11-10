<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { VList, type VListHandle } from 'virtua/svelte';
  import AnsiText from './AnsiText.svelte';
  import { ansiTokens } from '$lib/ansi.ts';
  import { useOutputUpdates } from '$lib/output-updates.ts';
  import { logTime } from '$lib/run-log.ts';
  import { trimOutputWindow } from '$lib/output-window.ts';
  import type { OutputEntry, OutputPage } from '$lib/log-page.ts';

  let { runId, activityId, running }: { runId: string; activityId?: string; running: boolean } =
    $props();
  let entries = $state.raw<OutputEntry[]>([]);
  let virtual = $state<VListHandle>();
  let generation = $state(0);
  let shift = $state(false);
  let loading = $state(false);
  let error = $state('');
  let following = $state(true);
  let copied = $state(false);
  let wrap = $state(true);
  let hasEarlier = $state(false);
  let hasLater = $state(false);
  let request: AbortController | undefined;
  let disposed = false;
  let initialized = false;
  let resumeAfter = 0;
  let lastOffset = 0;
  let pending = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  type LoadMode = 'tail' | 'start' | 'before' | 'after' | 'live';
  let retryMode: LoadMode = 'tail';
  const updates = useOutputUpdates();
  let download = $derived(
    `/api/runs/${encodeURIComponent(runId)}/logs?download=1${activityId ? `&activity=${encodeURIComponent(activityId)}` : ''}`
  );

  function refresh() {
    pending = true;
    if (!following) {
      hasLater = true;
      return;
    }
    if (loading || disposed || timer) return;
    timer = setTimeout(() => {
      timer = undefined;
      pending = false;
      void load(initialized ? 'live' : 'tail');
    }, 100);
  }

  function toBottom() {
    if (entries.length) virtual?.scrollToIndex(entries.length - 1, { align: 'end' });
  }

  // Virtua anchors a prepend or head eviction with shift=true. Apply insertion and
  // eviction in separate layout updates, even when their combined length is unchanged.
  async function extend(next: OutputPage, before: boolean) {
    shift = before;
    entries = before ? [...next.entries, ...entries] : [...entries, ...next.entries];
    await tick();
    if (disposed) return;
    const trimmed = trimOutputWindow(entries, before ? 'end' : 'start');
    if (before) {
      hasEarlier = next.hasEarlier;
      if (trimmed.removed) hasLater = true;
    } else {
      hasLater = next.hasLater;
      if (trimmed.removed) hasEarlier = true;
    }
    if (trimmed.removed) {
      shift = !before;
      entries = trimmed.entries;
      await tick();
    }
    resumeAfter = before ? (entries.at(-1)?.cursor ?? 0) : next.next;
  }

  // Requests belong to this reader. Reconnects retry the same cursor, and closing the
  // reader cancels its request and scheduled work. User scrolls pause tail fetching.
  async function load(mode: LoadMode) {
    if (disposed || loading) return;
    retryMode = mode;
    request?.abort();
    const controller = new AbortController();
    request = controller;
    loading = true;
    error = '';
    const params = new URLSearchParams();
    if (activityId) params.set('activity', activityId);
    if (mode === 'before' && entries.length) params.set('before', String(entries[0]!.cursor));
    if (mode === 'after' || mode === 'live') params.set('after', String(resumeAfter));
    if (mode === 'start') params.set('after', '0');
    if (mode === 'tail' || mode === 'start') following = mode === 'tail';
    try {
      const response = await fetch(`/api/runs/${encodeURIComponent(runId)}/logs?${params}`, {
        signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)])
      });
      if (!response.ok) throw new Error('Could not load output.');
      const next = (await response.json()) as OutputPage;
      if (disposed || request !== controller) return;
      if (mode === 'tail' || mode === 'start') {
        shift = false;
        entries = next.entries;
        hasEarlier = next.hasEarlier;
        hasLater = next.hasLater;
        resumeAfter = next.next;
        // Explicit jumps establish a new measurement origin; normal scrollback retains it.
        generation++;
        await tick();
        if (mode === 'start') virtual?.scrollToIndex(0, { align: 'start' });
      } else await extend(next, mode === 'before');
      initialized = true;
      if (disposed) return;
      if (following) toBottom();
      if (mode === 'live' && next.hasLater) pending = true;
    } catch (cause) {
      if (!disposed && request === controller && !controller.signal.aborted) {
        error = cause instanceof Error ? cause.message : String(cause);
        if (following) {
          clearTimeout(timer);
          timer = setTimeout(() => {
            timer = undefined;
            refresh();
          }, 2000);
        }
      }
    } finally {
      if (request === controller) {
        loading = false;
        lastOffset = virtual?.getScrollOffset() ?? 0;
        if (pending && following) refresh();
      }
    }
  }

  function scroll(offset: number) {
    if (!virtual || loading || !initialized) return;
    const remaining = virtual.getScrollSize() - virtual.getViewportSize() - offset;
    const direction = offset - lastOffset;
    lastOffset = offset;
    if (direction < 0 && remaining > 16) following = false;
    if (remaining <= 16 && !hasLater) following = true;
    if (!following && direction < 0 && offset < 400 && hasEarlier) void load('before');
    else if (direction > 0 && remaining < 400 && hasLater) void load('after');
  }

  function resized() {
    if (!loading && following) toBottom();
  }

  function rowText(entry: OutputEntry, index: number) {
    return (
      (activityId || entries[index - 1]?.more ? '' : `${logTime(entry.timestamp)} `) +
      (entry.text.endsWith('\n') ? entry.text.slice(0, -1) : entry.text)
    );
  }

  async function copy() {
    try {
      const text = entries
        .map(
          (entry, index) =>
            (activityId || entries[index - 1]?.more ? '' : `${logTime(entry.timestamp)} `) +
            entry.text +
            (entry.more ? '' : '\n')
        )
        .join('');
      await navigator.clipboard.writeText(
        ansiTokens(text)
          .map((token) => token.text)
          .join('')
      );
      copied = true;
    } catch {
      error = 'Could not copy the loaded output.';
    }
  }

  onMount(() => {
    const unsubscribe = updates.subscribe((id) => {
      if (id === runId) refresh();
    });
    void load('tail');
    return () => {
      disposed = true;
      clearTimeout(timer);
      unsubscribe();
      request?.abort();
    };
  });
</script>

<section
  class="flex min-h-0 flex-col gap-2"
  aria-label={activityId ? 'Activity output' : 'Run output'}
>
  <div class="flex flex-wrap items-center gap-1 text-xs">
    <button class="btn btn-ghost btn-xs" disabled={loading} onclick={() => load('start')}
      >Beginning</button
    >
    <button class="btn btn-ghost btn-xs" disabled={loading} onclick={() => load('tail')}
      >Latest output</button
    >
    <button class="btn btn-ghost btn-xs" disabled={!entries.length} onclick={copy}
      >{copied ? 'Copied' : 'Copy loaded output'}</button
    >
    <button class="btn btn-ghost btn-xs" aria-pressed={wrap} onclick={() => (wrap = !wrap)}
      >Wrap</button
    >
    <a class="btn btn-ghost btn-xs" href={download} download>Download log</a>
    <span class="ml-auto text-base-content/50"
      >{loading
        ? 'Loading output…'
        : following
          ? running
            ? 'Following live output'
            : 'Latest output'
          : 'Reading history'}</span
    >
  </div>
  {#if error}<p class="m-0 text-sm text-error" role="alert">
      {error} <button class="btn btn-ghost btn-xs" onclick={() => load(retryMode)}>Retry</button>
    </p>{/if}
  {#key generation}
    <VList
      bind:this={virtual}
      data={entries}
      getKey={(entry) => entry.cursor}
      {shift}
      style="height: 55vh; min-height: 8rem; overflow-anchor: none;"
      class="rounded border border-base-300 bg-base-200/30"
      role="region"
      aria-label="Log scrollback"
      tabindex={0}
      onscroll={scroll}
      onresize={resized}
      onwheel={(event) => {
        if (event.deltaY < 0) following = false;
      }}
    >
      {#snippet children(entry, index)}
        <pre
          data-output-cursor={entry.cursor}
          class={[
            'm-0 min-h-5 px-3 font-mono text-xs leading-5',
            wrap ? 'whitespace-pre-wrap wrap-anywhere' : 'whitespace-pre'
          ]}><AnsiText text={rowText(entry, index)} /></pre>
      {/snippet}
    </VList>
  {/key}
  {#if !loading && !entries.length}<p class="m-0 text-xs text-base-content/50">
      No recorded output.
    </p>{/if}
  <p class="m-0 text-xs text-base-content/50">
    {#if following}Scroll up to read earlier output.{:else}Scroll to load more output. Select Latest
      output to follow the run.{/if}
  </p>
</section>
