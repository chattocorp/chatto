<script lang="ts">
  import { Button, TextInput, TextArea } from '$lib/ui/form';

  let {
    name = $bindable(''),
    displayName = $bindable(''),
    description = $bindable(''),
    nameEditable = true,
    saving = false,
    submitLabel = 'Save',
    submitIcon = 'iconify uil--check',
    savingLabel = 'Saving...',
    onSubmit,
    onCancel
  }: {
    name?: string;
    displayName?: string;
    description?: string;
    nameEditable?: boolean;
    saving?: boolean;
    submitLabel?: string;
    submitIcon?: string;
    savingLabel?: string;
    onSubmit: () => void;
    onCancel?: () => void;
  } = $props();

  let nameError = $derived.by(() => {
    if (!name) return undefined;
    if (!/^[a-z]+$/.test(name)) {
      return 'Name must contain lowercase letters only';
    }
    if (name.length > 32) {
      return 'Name must be 32 characters or less';
    }
    return undefined;
  });

  let displayNameError = $derived.by(() => {
    if (!displayName) return undefined;
    if (displayName.length > 64) {
      return 'Display name must be 64 characters or less';
    }
    return undefined;
  });

  const isValid = $derived(name && displayName && !nameError && !displayNameError);

  function handleSubmit(e: Event) {
    e.preventDefault();
    if (isValid && !saving) {
      onSubmit();
    }
  }
</script>

<form onsubmit={handleSubmit} class="flex flex-col gap-4">
  {#if nameEditable}
    <TextInput
      id="name"
      testid="role-form-name"
      label="Name"
      bind:value={name}
      required
      disabled={saving}
      error={nameError}
      placeholder="e.g., moderator"
      description="Lowercase letters only. Cannot be changed after creation."
    />
  {:else}
    <div>
      <div class="mb-1 text-sm font-medium">Name</div>
      <code class="rounded bg-surface-200 px-2 py-1">{name}</code>
      <p class="mt-1 text-xs text-muted">Role names cannot be changed after creation.</p>
    </div>
  {/if}

  <TextInput
    id="displayName"
    testid="role-form-display-name"
    label="Display Name"
    bind:value={displayName}
    required
    disabled={saving}
    error={displayNameError}
    placeholder="e.g., Moderator"
  />

  <TextArea
    id="description"
    testid="role-form-description"
    label="Description"
    bind:value={description}
    rows={3}
    disabled={saving}
    placeholder="Optional description for this role"
  />

  <div class="flex gap-2 pt-2">
    <Button
      type="submit"
      variant="primary"
      disabled={!isValid || saving}
      loading={saving}
      loadingText={savingLabel}
    >
      {#if submitIcon}<span class={submitIcon}></span>{/if}
      {submitLabel}
    </Button>
    {#if onCancel}
      <Button type="button" variant="secondary" onclick={onCancel} disabled={saving}>
        <span class="iconify uil--times"></span>
        Cancel
      </Button>
    {/if}
  </div>
</form>
