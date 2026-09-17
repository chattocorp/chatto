<script lang="ts">
  import Select from './Select.svelte';
  let value = $state('first');
</script>

<Select
  id="bound-select"
  label="Device"
  options={[
    { value: 'first', label: 'First device' },
    { value: 'second', label: 'Second device' }
  ]}
  bind:value
/>
<output aria-label="Bound value">{value}</output>
