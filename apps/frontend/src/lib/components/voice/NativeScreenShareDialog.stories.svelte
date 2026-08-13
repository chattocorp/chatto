<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NativeScreenShareDialog from './NativeScreenShareDialog.svelte';

  const { Story } = defineMeta({
    title: 'Voice/Native screen share dialog',
    component: NativeScreenShareDialog,
    tags: ['autodocs']
  });

  const sources = [
    {
      id: 'window:moonring',
      kind: 'window' as const,
      applicationName: 'Moonring',
      bundleIdentifier: 'com.fluttermind.moonring',
      title: 'Moonring',
      width: 1920,
      height: 1080,
      preview: new Uint8Array()
    },
    {
      id: 'window:strategy',
      kind: 'window' as const,
      applicationName: 'Strategy Game',
      bundleIdentifier: 'example.strategy-game',
      title: 'Campaign — Turn 42',
      width: 2560,
      height: 1440,
      preview: new Uint8Array()
    },
    {
      id: 'display:1',
      kind: 'display' as const,
      displayIndex: 1,
      isMainDisplay: true,
      width: 3024,
      height: 1964,
      preview: new Uint8Array()
    },
    {
      id: 'display:2',
      kind: 'display' as const,
      displayIndex: 2,
      isMainDisplay: false,
      width: 2560,
      height: 1440,
      preview: new Uint8Array()
    }
  ];
</script>

<Story
  name="Sources"
  args={{
    visible: true,
    sources,
    loading: false,
    failed: false,
    onretry: () => {},
    onselect: () => {}
  }}
/>

<Story
  name="Loading"
  args={{
    visible: true,
    sources: [],
    loading: true,
    failed: false,
    onretry: () => {},
    onselect: () => {}
  }}
/>

<Story
  name="Empty"
  args={{
    visible: true,
    sources: [],
    loading: false,
    failed: false,
    onretry: () => {},
    onselect: () => {}
  }}
/>

<Story
  name="Failed"
  args={{
    visible: true,
    sources: [],
    loading: false,
    failed: true,
    onretry: () => {},
    onselect: () => {}
  }}
/>
