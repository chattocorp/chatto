<script lang="ts">
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import {
    deleteServerBanner,
    deleteServerLogo,
    getAuthenticatedServerState,
    updateServerConfig,
    uploadServerBanner,
    uploadServerLogo,
    type AuthenticatedServerState,
    type EditableServerConfig,
    type EditableServerProfile
  } from '@chatto/client/api/serverState';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, createQuery, queryClient } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';

  import { Panel, Hint, LoadingFog } from '$lib/ui';
  import { TextInput, TextArea, Button, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/dom/dropZone.svelte';
  import DropZoneOverlay from '$lib/dom/DropZoneOverlay.svelte';

  const MAX_SERVER_DESCRIPTION_BYTES = 500;

  const serverScope = useServerScope();
  const session = createSessionGuard(serverScope);

  type SettingsMutationScope = SessionSnapshot & {
    queryKey: ReturnType<typeof adminQueryKeys.serverSettings>;
  };

  type SaveVariables = SettingsMutationScope & { input: EditableServerConfig };
  type AssetOperation = 'upload-logo' | 'delete-logo' | 'upload-banner' | 'delete-banner';
  type AssetVariables = SettingsMutationScope & { operation: AssetOperation; file?: File };

  const settingsQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      queryKey: adminQueryKeys.serverSettings(serverId, connection),
      queryFn: ({ signal }) => getAuthenticatedServerState(connection.apiConfig, { signal }),
      refetchOnMount: 'always' as const
    };
  });

  const snapshot = $derived(settingsQuery.data ?? null);
  const loading = $derived(settingsQuery.isPending && snapshot === null);
  const loaded = $derived(snapshot?.viewerCanManageServer ?? false);

  // Explicit edits override query snapshots until saved or reverted.
  let draft = $state<Partial<EditableServerConfig>>({});
  const name = $derived(draft.name ?? snapshot?.name ?? '');
  const description = $derived(draft.description ?? snapshot?.description ?? '');
  const motd = $derived(draft.motd ?? snapshot?.motd ?? '');
  const welcomeMessage = $derived(draft.welcomeMessage ?? snapshot?.welcomeMessage ?? '');

  function edit(field: keyof EditableServerConfig, value: string) {
    // A pending save can replace the snapshot, so keep even a revert as an edit.
    draft[field] = !saving && value === (snapshot?.[field] ?? '') ? undefined : value;
  }

  // Logo state
  const logoUrl = $derived(snapshot?.logoUrl ?? null);
  let logoFileInput = $state<HTMLInputElement>();

  // Banner state
  const bannerUrl = $derived(snapshot?.bannerUrl ?? null);
  let bannerFileInput = $state<HTMLInputElement>();

  // Drag state
  let isDraggingLogo = $state(false);
  let isDraggingBanner = $state(false);

  // Validation
  let nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m('server_settings.name_empty');
    if (name !== name.trim()) return m('server_settings.name_trim');
    return undefined;
  });
  const changed = $derived(
    name !== (snapshot?.name ?? '') ||
      description !== (snapshot?.description ?? '') ||
      motd !== (snapshot?.motd ?? '') ||
      welcomeMessage !== (snapshot?.welcomeMessage ?? '')
  );

  $effect(() => {
    if (!snapshot || snapshot.viewerCanManageServer || !serverScope.isCurrent()) return;
    toast.error(m('server_settings.manage_denied'));
    goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverScope.serverId) }));
  });

  function mutationScope(): SettingsMutationScope {
    const snapshot = session.snapshot();
    return {
      ...snapshot,
      queryKey: adminQueryKeys.serverSettings(snapshot.serverId, snapshot.connection)
    };
  }

  function mergeEditableProfile(
    current: AuthenticatedServerState | undefined,
    profile: EditableServerProfile
  ): AuthenticatedServerState | undefined {
    return current
      ? {
          ...current,
          name: profile.name,
          description: profile.description,
          motd: profile.motd,
          welcomeMessage: profile.welcomeMessage
        }
      : current;
  }

  const saveMutation = createMutation(() => ({
    mutationFn: ({ connection, input }: SaveVariables) =>
      updateServerConfig(connection.apiConfig, input),
    onSuccess: (profile, variables) => {
      if (!session.isCurrent(variables)) return;
      queryClient.setQueryData<AuthenticatedServerState>(variables.queryKey, (current) =>
        mergeEditableProfile(current, profile)
      );

      // Clear only submitted values; a newer edit must survive the response.
      if (name.trim() === variables.input.name) delete draft.name;
      if (description.trim() === variables.input.description) delete draft.description;
      if (motd === variables.input.motd) delete draft.motd;
      if (welcomeMessage === variables.input.welcomeMessage) {
        delete draft.welcomeMessage;
      }
      toast.success(m('common.saved'));
    }
  }));

  function updateAssetSnapshot(variables: AssetVariables, profile: EditableServerProfile): void {
    queryClient.setQueryData<AuthenticatedServerState>(variables.queryKey, (current) => {
      if (!current) return current;
      if (variables.operation === 'upload-logo' || variables.operation === 'delete-logo') {
        return { ...current, logoUrl: profile.logoUrl };
      }
      return { ...current, bannerUrl: profile.bannerUrl };
    });
  }

  function assetSuccessMessage(operation: AssetOperation): string {
    switch (operation) {
      case 'upload-logo':
        return m('server_settings.logo_uploaded');
      case 'delete-logo':
        return m('server_settings.logo_removed');
      case 'upload-banner':
        return m('server_settings.banner_uploaded');
      case 'delete-banner':
        return m('server_settings.banner_removed');
    }
  }

  function assetErrorMessage(operation: AssetOperation): string {
    switch (operation) {
      case 'upload-logo':
        return m('server_settings.logo_upload_failed');
      case 'delete-logo':
        return m('server_settings.logo_delete_failed');
      case 'upload-banner':
        return m('server_settings.banner_upload_failed');
      case 'delete-banner':
        return m('server_settings.banner_delete_failed');
    }
  }

  const assetMutation = createMutation(() => ({
    mutationFn: ({ connection, operation, file }: AssetVariables) => {
      switch (operation) {
        case 'upload-logo':
          return uploadServerLogo(connection.apiConfig, file!);
        case 'delete-logo':
          return deleteServerLogo(connection.apiConfig);
        case 'upload-banner':
          return uploadServerBanner(connection.apiConfig, file!);
        case 'delete-banner':
          return deleteServerBanner(connection.apiConfig);
      }
    },
    onSuccess: (profile, variables) => {
      if (!session.isCurrent(variables)) return;
      updateAssetSnapshot(variables, profile);
      toast.success(assetSuccessMessage(variables.operation));
    },
    onError: (mutationError, variables) => {
      if (!session.isCurrent(variables)) return;
      toastError(mutationError, assetErrorMessage(variables.operation));
    },
    onSettled: (_profile, _error, variables) => {
      if (!session.isCurrent(variables)) return;
      if (variables.operation === 'upload-logo' && logoFileInput) logoFileInput.value = '';
      if (variables.operation === 'upload-banner' && bannerFileInput) bannerFileInput.value = '';
    }
  }));

  // Keep the form serialized even if a privacy generation fences the pending result.
  const saving = $derived(saveMutation.isPending);
  const activeAssetOperation = $derived(
    assetMutation.isPending && session.isCurrent(assetMutation.variables)
      ? assetMutation.variables.operation
      : null
  );
  const error = $derived.by(() => {
    if (settingsQuery.error) {
      return errorMessage(settingsQuery.error, m('server_settings.load_failed'));
    }
    if (saveMutation.isError && session.isCurrent(saveMutation.variables)) {
      return errorMessage(saveMutation.error, m('server_settings.save_failed'));
    }
    return null;
  });

  function handleSave(e: Event) {
    e.preventDefault();
    if (nameError || !changed || saving) return;
    saveMutation.mutate({
      ...mutationScope(),
      input: {
        name: name.trim(),
        description: description.trim(),
        motd,
        welcomeMessage
      }
    });
  }

  function uploadAsset(file: File | undefined, operation: 'upload-logo' | 'upload-banner') {
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      toast.error(m('server_settings.invalid_image'));
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m('server_settings.image_too_large'));
      return;
    }

    if (!assetMutation.isPending) {
      assetMutation.mutate({ ...mutationScope(), operation, file });
    }
  }

  const logoDropZone = dropZone({
    onDrop: (files) => uploadAsset(files[0], 'upload-logo'),
    onDragStateChange: (dragging) => (isDraggingLogo = dragging),
    acceptedTypes: ['image/*']
  });

  function handleLogoDelete() {
    if (!logoUrl || assetMutation.isPending) return;
    assetMutation.mutate({ ...mutationScope(), operation: 'delete-logo' });
  }

  const bannerDropZone = dropZone({
    onDrop: (files) => uploadAsset(files[0], 'upload-banner'),
    onDragStateChange: (dragging) => (isDraggingBanner = dragging),
    acceptedTypes: ['image/*']
  });

  function handleBannerDelete() {
    if (!bannerUrl || assetMutation.isPending) return;
    assetMutation.mutate({ ...mutationScope(), operation: 'delete-banner' });
  }
</script>

{#if loading}
  <LoadingFog class="h-64 w-full" label={m('server_settings.loading')} />
{:else if loaded}
  <div class="flex flex-col gap-6">
    <!-- Server Details Form -->
    <Panel title={m('server_settings.general')} icon="iconify icon-[uil--edit]">
      <form onsubmit={handleSave} class="flex flex-col gap-4">
        <TextInput
          id="name"
          label={m('server_settings.name_label')}
          bind:value={() => name, (value) => edit('name', value)}
          required
          disabled={saving}
          error={nameError}
        />

        <TextArea
          id="description"
          label={m('server_settings.description_label')}
          bind:value={() => description, (value) => edit('description', value)}
          maxBytes={MAX_SERVER_DESCRIPTION_BYTES}
          disabled={saving}
          rows={2}
          description={m('server_settings.description_help')}
        />

        <TextInput
          id="motd"
          label={m('common.message_of_the_day')}
          bind:value={() => motd, (value) => edit('motd', value)}
          disabled={saving}
          description={m('server_settings.motd_help')}
        />

        <TextArea
          id="welcome-message"
          label={m('server_settings.welcome_message_label')}
          bind:value={() => welcomeMessage, (value) => edit('welcomeMessage', value)}
          rows={3}
          disabled={saving}
          description={m('server_settings.welcome_message_help')}
        />

        {#if error}
          <FormError {error} />
        {/if}

        <div class="flex items-center gap-3">
          <Button
            type="submit"
            loading={saving}
            disabled={!changed || !name.trim() || !!nameError}
            loadingText={m('server_settings.saving')}
          >
            <span aria-hidden="true" class="iconify icon-[uil--check]"></span>
            {m('server_settings.save_button')}
          </Button>
        </div>
      </form>
    </Panel>

    <!-- Logo Section -->
    <Panel title={m('server_settings.logo')} icon="iconify icon-[uil--image]">
      <div
        class="relative flex items-start gap-6"
        data-testid="logo-drop-zone"
        {@attach logoDropZone}
      >
        <DropZoneOverlay
          visible={isDraggingLogo}
          title={m('server_settings.drop_image')}
          subtitle={m('server_settings.logo_drop_subtitle')}
        />
        <!-- Logo Preview -->
        <div
          class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface text-5xl font-black text-muted"
        >
          {#if logoUrl}
            <img
              src={logoUrl}
              alt={m('server_settings.logo_alt')}
              class="h-full w-full object-cover"
            />
          {:else}
            {name?.[0]?.toUpperCase() || '?'}
          {/if}
        </div>

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            {m('server_settings.logo_description')}
          </p>
          <div class="flex gap-2">
            <input
              type="file"
              accept="image/*"
              class="hidden"
              bind:this={logoFileInput}
              onchange={(event) => uploadAsset(event.currentTarget.files?.[0], 'upload-logo')}
            />
            <Button
              onclick={() => logoFileInput?.click()}
              loading={activeAssetOperation === 'upload-logo'}
              disabled={assetMutation.isPending}
              loadingText={m('server_settings.uploading')}
            >
              <span class="inline-flex items-center gap-2">
                <span aria-hidden="true" class="iconify icon-[uil--image-upload]"></span>
                {logoUrl ? m('server_settings.logo_change') : m('server_settings.logo_upload')}
              </span>
            </Button>
            {#if logoUrl}
              <Button
                variant="danger-secondary"
                onclick={handleLogoDelete}
                loading={activeAssetOperation === 'delete-logo'}
                disabled={assetMutation.isPending}
                loadingText={m('server_settings.removing')}
              >
                <span class="inline-flex items-center gap-2">
                  <span aria-hidden="true" class="iconify icon-[uil--trash-alt]"></span>
                  {m('server_settings.remove')}
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>

    <!-- Banner Section -->
    <Panel title={m('server_settings.banner')} icon="iconify icon-[uil--scenery]">
      <div
        class="relative flex flex-col gap-4"
        data-testid="banner-drop-zone"
        {@attach bannerDropZone}
      >
        <DropZoneOverlay
          visible={isDraggingBanner}
          title={m('server_settings.drop_image')}
          subtitle={m('server_settings.banner_drop_subtitle')}
        />
        <!-- Banner Preview — capped width so the OG-aspect 1200×630 doesn't
             swallow the panel on wide layouts. -->
        {#if bannerUrl}
          <div class="w-full max-w-md overflow-hidden rounded-lg bg-surface">
            <img
              src={bannerUrl}
              alt={m('server_settings.banner_alt')}
              class="aspect-[1200/630] w-full object-cover"
            />
          </div>
        {:else}
          <div
            class="flex aspect-[1200/630] w-full max-w-md items-center justify-center rounded-lg border-2 border-dashed border-border bg-surface text-muted"
          >
            <span class="text-sm">{m('server_settings.no_banner')}</span>
          </div>
        {/if}

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            {m('server_settings.banner_description')}
          </p>
          <div class="flex gap-2">
            <input
              type="file"
              accept="image/*"
              class="hidden"
              bind:this={bannerFileInput}
              onchange={(event) => uploadAsset(event.currentTarget.files?.[0], 'upload-banner')}
            />
            <Button
              onclick={() => bannerFileInput?.click()}
              loading={activeAssetOperation === 'upload-banner'}
              disabled={assetMutation.isPending}
              loadingText={m('server_settings.uploading')}
            >
              <span class="inline-flex items-center gap-2">
                <span aria-hidden="true" class="iconify icon-[uil--image-upload]"></span>
                {bannerUrl
                  ? m('server_settings.banner_change')
                  : m('server_settings.banner_upload')}
              </span>
            </Button>
            {#if bannerUrl}
              <Button
                variant="danger-secondary"
                onclick={handleBannerDelete}
                loading={activeAssetOperation === 'delete-banner'}
                disabled={assetMutation.isPending}
                loadingText={m('server_settings.removing')}
              >
                <span class="inline-flex items-center gap-2">
                  <span aria-hidden="true" class="iconify icon-[uil--trash-alt]"></span>
                  {m('server_settings.remove')}
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>
  </div>
{:else if error}
  <Hint tone="danger">{error}</Hint>
{/if}
