<!--
@component

Desktop game-capture source picker. The host supplies temporary opaque source
IDs; this component only presents window metadata and reports explicit choice.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { GameCaptureSource } from '$lib/desktop/gameCapture';
  import Dialog from '$lib/ui/Dialog.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { Button, TextInput } from '$lib/ui/form';

  let {
    visible = $bindable(false),
    sources,
    loading,
    failed,
    onretry,
    onselect
  }: {
    visible?: boolean;
    sources: GameCaptureSource[];
    loading: boolean;
    failed: boolean;
    onretry: () => void;
    onselect: (source: GameCaptureSource) => void;
  } = $props();

  const id = $props.id();
  const descriptionId = `${id}-description`;
  let query = $state('');
  let filteredSources = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return sources;
    return sources.filter((source) =>
      `${source.applicationName}\n${source.title}`.toLocaleLowerCase().includes(needle)
    );
  });
  let sourceGroups = $derived.by(() => {
    const groups: Array<{
      bundleIdentifier: string;
      applicationName: string;
      sources: GameCaptureSource[];
    }> = [];
    for (const source of filteredSources) {
      const group = groups.find(
        (candidate) => candidate.bundleIdentifier === source.bundleIdentifier
      );
      if (group) group.sources.push(source);
      else {
        groups.push({
          bundleIdentifier: source.bundleIdentifier,
          applicationName: source.applicationName,
          sources: [source]
        });
      }
    }
    return groups;
  });

  function close() {
    visible = false;
  }

  function handleClose() {
    query = '';
  }
</script>

{#snippet footer()}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={close}>{m('common.cancel')}</Button>
  </div>
{/snippet}

<Dialog
  bind:visible
  title={m('voice.stream_game')}
  size="lg"
  describedBy={descriptionId}
  onclose={handleClose}
  {footer}
>
  <p id={descriptionId} class="text-muted">
    {m('voice.stream_game_description')}
  </p>

  {#if loading}
    <div
      class="mt-4 flex flex-col gap-2 rounded-lg bg-background p-1"
      aria-label={m('common.loading')}
      role="status"
    >
      {#each Array(4) as _, index (index)}
        <div class="flex items-center gap-3 rounded-md p-3">
          <div class="skeleton size-10 shrink-0 rounded-md"></div>
          <div class="flex min-w-0 flex-1 flex-col gap-2">
            <div class="skeleton h-4 w-1/3 rounded"></div>
            <div class="skeleton h-4 w-2/3 rounded"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else if failed}
    <div class="mt-4 flex min-h-48 flex-col rounded-lg bg-background" role="alert">
      <EmptyState icon="icon-[uil--exclamation-circle]" title={m('voice.game_windows_failed')}>
        <Button variant="secondary" size="sm" onclick={onretry}>{m('common.retry')}</Button>
      </EmptyState>
    </div>
  {:else if sources.length === 0}
    <div class="mt-4 flex min-h-48 flex-col rounded-lg bg-background">
      <EmptyState icon="icon-[uil--window-section]">{m('voice.no_game_windows')}</EmptyState>
    </div>
  {:else}
    <div class="mt-4">
      <TextInput
        id={`${id}-search`}
        label={m('search.action')}
        labelHidden
        bind:value={query}
        placeholder={m('search.action')}
        leadingIcon="iconify icon-[uil--search]"
        autocomplete="off"
      />
    </div>

    {#if sourceGroups.length === 0}
      <div class="mt-3 flex min-h-40 flex-col rounded-lg bg-background">
        <EmptyState icon="icon-[uil--search]">{m('quick_switcher.no_results')}</EmptyState>
      </div>
    {:else}
      <div class="mt-3 max-h-[48vh] overflow-y-auto rounded-lg bg-background p-1">
        {#each sourceGroups as group (group.bundleIdentifier)}
          <section aria-labelledby={`${id}-${group.bundleIdentifier}`}>
            <h3
              id={`${id}-${group.bundleIdentifier}`}
              class="flex items-center gap-2 px-3 py-2 font-medium text-text-top"
            >
              <span
                class="grid size-8 shrink-0 place-items-center rounded-md bg-surface text-muted"
                aria-hidden="true"
              >
                <span class="iconify icon-[uil--apps] text-lg"></span>
              </span>
              <bdi class="truncate">{group.applicationName}</bdi>
            </h3>
            <ul class="selectable-list" aria-label={group.applicationName}>
              {#each group.sources as source (source.id)}
                <li>
                  <button
                    type="button"
                    class="flex w-full cursor-pointer items-center gap-3 selectable-list-item px-3 py-2.5 text-start"
                    onclick={() => onselect(source)}
                  >
                    <span
                      class="iconify icon-[uil--window] shrink-0 text-xl text-muted"
                      aria-hidden="true"
                    ></span>
                    <span class="min-w-0 flex-1 truncate font-medium">
                      <bdi>{source.title || m('voice.untitled_window')}</bdi>
                    </span>
                    <span class="hidden shrink-0 text-muted tabular-nums sm:inline">
                      {source.width}×{source.height}
                    </span>
                    <span
                      class="iconify icon-[uil--angle-right] shrink-0 text-xl text-muted rtl:rotate-180"
                      aria-hidden="true"
                    ></span>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {/if}
  {/if}
</Dialog>
