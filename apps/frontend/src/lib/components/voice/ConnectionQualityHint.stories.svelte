<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ConnectionQualityHint from './ConnectionQualityHint.svelte';
  const { Story } = defineMeta({
    title: 'Voice/Connection quality',
    component: ConnectionQualityHint,
    tags: ['autodocs']
  });
</script>

<Story name="Poor connection" args={{ quality: 'poor' }} />
<Story name="Connection lost" args={{ quality: 'lost' }} />
<Story name="Healthy (hidden)" args={{ quality: 'excellent' }} />
<Story name="Unknown (hidden)" args={{ quality: 'unknown' }} />
