<script lang="ts">
  import { formatPageTitle } from '$lib/render/pageTitle';

  let { title = '', scope = 'route' }: { title?: string; scope?: 'route' | 'app' } = $props();
</script>

<!-- @component Renders the document title. Use scope="app" for product-wide pages and omit title for a server/product fallback. Render only one PageTitle at a time. -->
<svelte:head>
  <title>{formatPageTitle(title, scope)}</title>
</svelte:head>
