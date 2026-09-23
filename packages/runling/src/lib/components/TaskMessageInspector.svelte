<script lang="ts">
  import { onMount } from "svelte";
  import { duration } from "$lib/runs.ts";
  import type { TaskMessage } from "$lib/timeline.ts";

  let { messages, selected, onselect, onclose }: {
    messages: TaskMessage[];
    selected: TaskMessage;
    onselect: (id: string) => void;
    onclose: () => void;
  } = $props();

  let dialog: HTMLDialogElement;
  const titleId = $props.id();

  // Use the top layer so details stay visible in both normal and expanded timelines.
  onMount(() => dialog.showModal());
</script>

<dialog class="modal modal-middle" bind:this={dialog} {onclose} aria-labelledby={titleId}>
  <div class="modal-box flex max-h-[85dvh] w-11/12 max-w-3xl flex-col overflow-hidden p-0">
    <header class="flex items-start justify-between gap-4 border-b border-base-300 px-6 py-4">
      <div>
        <h2 id={titleId} class="text-lg font-medium">Task message</h2>
        <p class="mt-1 text-xs text-base-content/60">{selected.status} · {duration(selected.timestamp)}</p>
      </div>
      <button class="btn btn-ghost btn-sm btn-square" onclick={() => dialog.close()} aria-label="Close task message">
        <span class="icon-[lucide--x] size-4" aria-hidden="true"></span>
      </button>
    </header>

    <div class="min-h-0 space-y-4 overflow-auto px-6 py-5">
      {#if messages.length > 1}
        <label class="block text-xs text-base-content/60">
          Message
          <select class="select select-sm mt-1 w-full text-base-content" aria-label="Select task message" value={selected.id} onchange={event => onselect(event.currentTarget.value)}>
            {#each messages as item (item.id)}
              <option value={item.id}>{duration(item.timestamp)} · {item.payload.slice(0, 100)}</option>
            {/each}
          </select>
        </label>
      {/if}

      <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt class="text-base-content/60">From</dt><dd class="min-w-0 wrap-anywhere">{selected.from}</dd>
        <dt class="text-base-content/60">To</dt><dd class="min-w-0 wrap-anywhere">{selected.to}</dd>
      </dl>
      <pre class="m-0 whitespace-pre-wrap font-mono text-sm leading-relaxed wrap-anywhere">{selected.payload}</pre>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop">
    <button aria-label="Close task message">Close</button>
  </form>
</dialog>
