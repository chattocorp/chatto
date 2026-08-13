<!--
@component

One screen-share control for both web and native hosts. An optional native host
capability opens Chatto's source chooser; without it, the same click delegates
to the browser's `getDisplayMedia` picker through LiveKit.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';
  import {
    cancelNativeScreenShareSourceList,
    isNativeScreenShareAvailable,
    listNativeScreenShareSources,
    type NativeScreenShareSource
  } from '$lib/desktop/nativeScreenShare';
  import { toast } from '$lib/ui/toast';
  import NativeScreenShareDialog from './NativeScreenShareDialog.svelte';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';

  let {
    voiceCallState,
    class: className,
    testId,
    iconClass
  }: {
    voiceCallState: VoiceCallState;
    class: string;
    testId: string;
    iconClass?: string;
  } = $props();
  const serverScope = useServerScope();

  let nativeAvailable = $state(false);
  let dialogVisible = $state(false);
  let sources = $state.raw<NativeScreenShareSource[]>([]);
  let sourcesLoading = $state(false);
  let sourcesFailed = $state(false);
  let sourceRequestGeneration = 0;
  let pending = $derived(
    sourcesLoading ||
      voiceCallState.isScreenSharePending ||
      voiceCallState.isNativeScreenSharePending
  );

  onMount(() => {
    nativeAvailable = isNativeScreenShareAvailable();
  });

  async function refreshSources() {
    const generation = ++sourceRequestGeneration;
    sourcesLoading = true;
    sourcesFailed = false;
    try {
      const nextSources = await listNativeScreenShareSources();
      if (generation === sourceRequestGeneration && dialogVisible) sources = nextSources;
    } catch {
      if (generation !== sourceRequestGeneration || !dialogVisible) return;
      sources = [];
      sourcesFailed = true;
    } finally {
      if (generation === sourceRequestGeneration) sourcesLoading = false;
    }
  }

  function clearChooserState(preserveSelectedOffer = false) {
    sourceRequestGeneration += 1;
    if (!preserveSelectedOffer) cancelNativeScreenShareSourceList();
    sources = [];
    sourcesLoading = false;
    sourcesFailed = false;
  }

  function openNativeChooser() {
    dialogVisible = true;
    void refreshSources();
  }

  async function handleControl() {
    try {
      if (voiceCallState.isScreenShareEnabled) {
        await voiceCallState.toggleScreenShare();
        return;
      }
      if (nativeAvailable) {
        openNativeChooser();
        return;
      }
      await voiceCallState.toggleScreenShare();
    } catch {
      if (!serverScope.isCurrent()) return;
      toast.error(m('voice.screen_share_failed'));
    }
  }

  async function selectSource(source: NativeScreenShareSource) {
    dialogVisible = false;
    clearChooserState(true);
    const sourceName =
      source.kind === 'window'
        ? source.title || source.applicationName
        : source.isMainDisplay
          ? m('voice.main_display')
          : m('voice.display_number', { number: source.displayIndex });
    try {
      await voiceCallState.startNativeScreenShare(source.id, sourceName);
    } catch {
      if (!serverScope.isCurrent()) return;
      toast.error(m('voice.screen_share_failed'));
    }
  }
</script>

<VoiceCallControlButton
  class={className}
  label={voiceCallState.isScreenShareEnabled
    ? m('voice.stop_share_screen')
    : m('voice.share_screen')}
  {testId}
  icon="icon-[uil--desktop]"
  {iconClass}
  onclick={() => void handleControl()}
  {pending}
/>

{#if nativeAvailable}
  <NativeScreenShareDialog
    bind:visible={dialogVisible}
    {sources}
    loading={sourcesLoading}
    failed={sourcesFailed}
    onretry={() => void refreshSources()}
    onselect={selectSource}
    ondismiss={clearChooserState}
  />
{/if}
