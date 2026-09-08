<script module lang="ts">
  export type BotCredentialSectionItem = {
    id: string;
    name: string;
    createdAt: string;
    lastUsed: string;
  };

  export type BotCredentialSectionLabels = {
    title: string;
    description: string;
    create: string;
    name: string;
    createdAt: string;
    lastUsed: string;
    empty: string;
    limitReached: string;
    revoke: string;
    revokeWarning: string;
    issuedTitle: string;
    issuedWarning: string;
    copied: string;
  };
</script>

<script lang="ts">
  import ShowOnceCredentialDialog from './ShowOnceCredentialDialog.svelte';
  import { ConfirmDialog, FormDialog } from '$lib/ui';
  import BotIntegrationSection from './BotIntegrationSection.svelte';
  import BotIntegrationDetails from './BotIntegrationDetails.svelte';
  import { Button, TextInput } from '$lib/ui/form';

  let {
    idPrefix,
    testId,
    items,
    labels,
    createIcon,
    limit = 20,
    oncreate,
    onrevoke
  }: {
    idPrefix: string;
    testId: string;
    items: BotCredentialSectionItem[];
    labels: BotCredentialSectionLabels;
    createIcon: string;
    limit?: number;
    /** Returns the show-once credential, or null when creation did not complete for this target. */
    oncreate: (name: string) => Promise<string | null>;
    /** Returns true when revocation completed for this target. */
    onrevoke: (id: string) => Promise<boolean>;
  } = $props();

  let createVisible = $state(false);
  let createName = $state('');
  let createLoading = $state(false);
  let issuedVisible = $state(false);
  let issuedValue = $state('');
  let revokeVisible = $state(false);
  let revokeId = $state('');
  let revokeLoading = $state(false);

  const normalizedName = $derived(createName.trim());
  const atLimit = $derived(items.length >= limit);

  function openCreate() {
    createName = '';
    createVisible = true;
  }

  async function createCredential() {
    if (!normalizedName) return;
    createLoading = true;
    try {
      const value = await oncreate(normalizedName);
      if (!value) return;
      createVisible = false;
      issuedValue = value;
      issuedVisible = true;
    } finally {
      createLoading = false;
    }
  }

  function openRevoke(id: string) {
    revokeId = id;
    revokeVisible = true;
  }

  async function revokeCredential() {
    if (!revokeId) return;
    revokeLoading = true;
    try {
      if (await onrevoke(revokeId)) revokeVisible = false;
    } finally {
      revokeLoading = false;
    }
  }
</script>

<!-- @component Manages one named bot-credential collection, including create, show-once, and revoke dialogs. -->
<BotIntegrationSection
  title={labels.title}
  description={labels.description}
  {testId}
  {items}
  empty={labels.empty}
>
  {#snippet actions()}
    <Button size="sm" disabled={atLimit} onclick={openCreate}>
      <span class={createIcon} aria-hidden="true"></span>
      {labels.create}
    </Button>
  {/snippet}
  {#snippet details(item)}
    <BotIntegrationDetails name={item.name}>
      <div>
        <dt class="text-muted">{labels.createdAt}</dt>
        <dd>{item.createdAt}</dd>
      </div>
      <div>
        <dt class="text-muted">{labels.lastUsed}</dt>
        <dd>{item.lastUsed}</dd>
      </div>
    </BotIntegrationDetails>
  {/snippet}
  {#snippet itemActions(item)}
    <Button
      size="sm"
      variant="danger-secondary"
      label={labels.revoke}
      title={labels.revoke}
      onclick={() => openRevoke(item.id)}
    >
      <span class="iconify icon-[uil--times-circle]" aria-hidden="true"></span>
    </Button>
  {/snippet}
  {#snippet footer()}
    {#if atLimit}<div class="border-t border-border px-5 py-3 text-muted">
        {labels.limitReached}
      </div>{/if}
  {/snippet}
</BotIntegrationSection>

<FormDialog
  bind:visible={createVisible}
  title={labels.create}
  submitLabel={labels.create}
  loading={createLoading}
  disabled={!normalizedName}
  onsubmit={createCredential}
  onclose={() => (createVisible = false)}
>
  <TextInput
    id={`create-${idPrefix}-name`}
    label={labels.name}
    maxlength={64}
    required
    bind:value={createName}
  />
</FormDialog>

<ShowOnceCredentialDialog
  bind:visible={issuedVisible}
  bind:value={issuedValue}
  pending={createLoading}
  title={labels.issuedTitle}
  warning={labels.issuedWarning}
  copiedMessage={labels.copied}
/>

<ConfirmDialog
  bind:visible={revokeVisible}
  title={labels.revoke}
  actionLabel={labels.revoke}
  loading={revokeLoading}
  onconfirm={revokeCredential}
  onclose={() => (revokeVisible = false)}
>
  {labels.revokeWarning}
</ConfirmDialog>
