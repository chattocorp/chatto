<script lang="ts">
  import FormField from './FormField.svelte';

  type Preset = '24h' | '7d' | '30d' | 'indefinite' | 'custom';

  let {
    id,
    label = 'Expires',
    value = $bindable<string | null>(null),
    valid = $bindable(true),
    disabled = false
  }: {
    id: string;
    label?: string;
    value?: string | null;
    valid?: boolean;
    disabled?: boolean;
  } = $props();

  let preset = $state<Preset>('indefinite');
  let customLocal = $state('');

  const presets: { value: Preset; label: string }[] = [
    { value: '24h', label: '24 hours' },
    { value: '7d', label: '7 days' },
    { value: '30d', label: '30 days' },
    { value: 'indefinite', label: 'No expiry' },
    { value: 'custom', label: 'Custom' }
  ];

  const customError = $derived.by(() => {
    if (preset !== 'custom') return null;
    if (!customLocal) return 'Choose an expiry date.';
    const date = new Date(customLocal);
    if (Number.isNaN(date.getTime())) return 'Enter a valid expiry.';
    if (date <= new Date()) return 'Expiry must be in the future.';
    return null;
  });

  function isoAfter(ms: number): string {
    return new Date(Date.now() + ms).toISOString();
  }

  function updateValue() {
    switch (preset) {
      case '24h':
        value = isoAfter(24 * 60 * 60 * 1000);
        break;
      case '7d':
        value = isoAfter(7 * 24 * 60 * 60 * 1000);
        break;
      case '30d':
        value = isoAfter(30 * 24 * 60 * 60 * 1000);
        break;
      case 'custom':
        value = customError ? null : new Date(customLocal).toISOString();
        break;
      case 'indefinite':
        value = null;
        break;
    }
    valid = !customError;
  }
</script>

<div class="flex flex-col gap-3">
  <FormField {id} {label}>
    <select
      {id}
      bind:value={preset}
      {disabled}
      class="input"
      onchange={updateValue}
    >
      {#each presets as option (option.value)}
        <option value={option.value}>{option.label}</option>
      {/each}
    </select>
  </FormField>

  {#if preset === 'custom'}
    <FormField
      id={`${id}-custom`}
      label="Custom expiry"
      error={customError ?? undefined}
    >
      <input
        id={`${id}-custom`}
        class="input"
        type="datetime-local"
        bind:value={customLocal}
        {disabled}
        aria-invalid={customError ? 'true' : undefined}
        aria-describedby={customError ? `${id}-custom-error` : undefined}
        oninput={updateValue}
      />
    </FormField>
  {/if}
</div>
