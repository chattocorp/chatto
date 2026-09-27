<!--
@component
Test harness for DataTable row links: one row with a row link, passive text,
and an independent button.
-->
<script lang="ts">
  import DataTable from './DataTable.svelte';

  let { linked = true }: { linked?: boolean } = $props();
</script>

<DataTable items={[{ id: '1' }]} columns={3}>
  {#snippet header()}
    <th class="table-header-cell">Name</th>
    <th class="table-header-cell">Note</th>
    <th class="table-header-cell">Actions</th>
  {/snippet}
  {#snippet row()}
    <td class="px-4 py-3">
      {#if linked}
        <a class="data-table-row-link" href="#member-1">Alice</a>
      {:else}
        Alice
      {/if}
    </td>
    <td class="px-4 py-3" data-testid="note">Long note text</td>
    <td class="px-4 py-3">
      <button type="button" data-testid="copy">Copy</button>
    </td>
  {/snippet}
</DataTable>
