<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import GameCaptureSourceDialog from './GameCaptureSourceDialog.svelte';

  const { Story } = defineMeta({
    title: 'Voice/Game capture source dialog',
    component: GameCaptureSourceDialog,
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
      height: 1080
    },
    {
      id: 'window:strategy',
      kind: 'window' as const,
      applicationName: 'Strategy Game',
      bundleIdentifier: 'example.strategy-game',
      title: 'Campaign — Turn 42',
      width: 2560,
      height: 1440
    },
    {
      id: 'window:strategy-map',
      kind: 'window' as const,
      applicationName: 'Strategy Game',
      bundleIdentifier: 'example.strategy-game',
      title: 'World map',
      width: 1920,
      height: 1080
    }
  ];
</script>

<Story
  name="Windows"
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
