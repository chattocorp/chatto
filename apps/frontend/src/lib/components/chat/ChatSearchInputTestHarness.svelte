<script lang="ts">
  import ChatSearchInput from './ChatSearchInput.svelte';

  let {
    value = $bindable(''),
    focusOnMount = false,
    disabled = false
  }: { value?: string; focusOnMount?: boolean; disabled?: boolean } = $props();

  let inputCount = $state(0);
  let submitCount = $state(0);
  let clearCount = $state(0);
</script>

<ChatSearchInput
  id="chat-search-test"
  label="Search messages"
  placeholder="Find messages"
  clearLabel="Clear search"
  bind:value
  {focusOnMount}
  {disabled}
  oninput={() => inputCount++}
  onsubmit={() => submitCount++}
  onclear={() => clearCount++}
/>

<output data-testid="search-value">{value}</output>
<output data-testid="input-count">{inputCount}</output>
<output data-testid="submit-count">{submitCount}</output>
<output data-testid="clear-count">{clearCount}</output>
