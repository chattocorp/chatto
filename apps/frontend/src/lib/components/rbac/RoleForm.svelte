<script lang="ts">
  import { Button, Checkbox, Form, TextInput, TextArea } from '$lib/ui/form';
  import * as m from '$lib/i18n/messages';

  let {
    name = $bindable(''),
    displayName = $bindable(''),
    description = $bindable(''),
    pingable = $bindable(false),
    nameEditable = true,
    saving = false,
    submitLabel = m['rbac.role_form.save'](),
    submitIcon = 'iconify uil--check',
    savingLabel = m['rbac.role_form.saving'](),
    onSubmit,
    onCancel
  }: {
    name?: string;
    displayName?: string;
    description?: string;
    pingable?: boolean;
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
    if (name.length > 32) {
      return m['rbac.role_form.name_too_long']();
    }
    if (!/^[a-z]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
      return m['rbac.role_form.name_invalid']();
    }
    return undefined;
  });

  let displayNameError = $derived.by(() => {
    if (!displayName) return undefined;
    if (displayName.length > 64) {
      return m['rbac.role_form.display_name_too_long']();
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

<Form onsubmit={handleSubmit}>
  {#if nameEditable}
    <TextInput
      id="name"
      testid="role-form-name"
      label={m['rbac.role_form.name']()}
      bind:value={name}
      required
      disabled={saving}
      error={nameError}
      placeholder={m['rbac.role_form.name_placeholder']()}
      description={m['rbac.role_form.name_description']()}
    />
  {:else}
    <div>
      <div class="mb-1 text-sm font-medium">{m['rbac.role_form.name']()}</div>
      <code class="rounded bg-surface-200 px-2 py-1">{name}</code>
      <p class="mt-1 text-xs text-muted">{m['rbac.role_form.name_locked']()}</p>
    </div>
  {/if}

  <TextInput
    id="displayName"
    testid="role-form-display-name"
    label={m['rbac.role_form.display_name']()}
    bind:value={displayName}
    required
    disabled={saving}
    error={displayNameError}
    placeholder={m['rbac.role_form.display_name_placeholder']()}
  />

  <TextArea
    id="description"
    testid="role-form-description"
    label={m['rbac.role_form.description']()}
    bind:value={description}
    rows={3}
    disabled={saving}
    placeholder={m['rbac.role_form.description_placeholder']()}
  />

  <Checkbox
    id="pingable"
    bind:checked={pingable}
    label={m['rbac.role_form.pingable']()}
    disabled={saving}
    description={m['rbac.role_form.pingable_description']()}
  />

  {#snippet footer()}
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
        {m['common.cancel']()}
      </Button>
    {/if}
  {/snippet}
</Form>
