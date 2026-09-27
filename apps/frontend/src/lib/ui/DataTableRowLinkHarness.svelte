<!--
@component
Test harness for DataTable row links: three rows, each with a row link, passive
text, and an independent button. Link and button clicks are reported through
callbacks instead of navigating.
-->
<script lang="ts">
  import DataTable from './DataTable.svelte';

  let {
    linked = true,
    onnavigate = () => {},
    oncopy = () => {}
  }: {
    linked?: boolean;
    onnavigate?: (id: string) => void;
    oncopy?: (id: string) => void;
  } = $props();

  const items = [
    { id: '1', name: 'Alice' },
    { id: '2', name: 'Bob' },
    { id: '3', name: 'Carol' }
  ];
</script>

<DataTable {items} columns={3}>
  {#snippet header()}
    <th class="table-header-cell">Name</th>
    <th class="table-header-cell">Note</th>
    <th class="table-header-cell">Actions</th>
  {/snippet}
  {#snippet row(item)}
    <td class="px-4 py-3">
      {#if linked}
        <a
          class="data-table-row-link"
          href={`#member-${item.id}`}
          onclick={(event) => {
            event.preventDefault();
            onnavigate(item.id);
          }}>{item.name}</a
        >
      {:else}
        {item.name}
      {/if}
    </td>
    <td class="px-4 py-3 select-text" data-testid={`note-${item.id}`}>Note for {item.name}</td>
    <td class="px-4 py-3">
      <button type="button" data-testid={`copy-${item.id}`} onclick={() => oncopy(item.id)}
        >Copy</button
      >
    </td>
  {/snippet}
</DataTable>
