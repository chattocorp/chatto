<script lang="ts">
  import type { AnyComponent } from 'gpuix-svelte';
  import Theme from '../src/ui/Theme.svelte';
  let {
    component: Content,
    componentProps
  }: { component: AnyComponent; componentProps: Record<string, unknown> } = $props();
</script>

<Theme><Content {...componentProps} /></Theme>
