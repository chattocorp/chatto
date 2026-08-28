<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { onDestroy, untrack } from 'svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
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
  } from '$lib/api-client/serverState';
  import { adminQueryKeys } from '$lib/query/admin';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { queryClient } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';

  import Panel from '$lib/ui/Panel.svelte';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import FormError from '$lib/ui/form/FormError.svelte';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  const MAX_SERVER_DESCRIPTION_BYTES = 500;

  const serverScope = useServerScope();
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  type SettingsMutationScope = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof adminQueryKeys.serverSettings>;
    privacyGeneration: number;
  };

  type SaveVariables = SettingsMutationScope & { input: EditableServerConfig };
  type AssetOperation = 'upload-logo' | 'delete-logo' | 'upload-banner' | 'delete-banner';
  type AssetVariables = SettingsMutationScope & { operation: AssetOperation; file?: File };

  const settingsQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.serverSettings(serverId, connection),
        queryFn: ({ signal }) => getAuthenticatedServerState(connection.apiConfig, { signal }),
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );

  const snapshot = $derived(settingsQuery.data ?? null);
  const loading = $derived(settingsQuery.isPending && snapshot === null);
  const loaded = $derived(snapshot?.viewerCanManageServer ?? false);

  // Form state
  let name = $state('');
  let description = $state('');
  let motd = $state('');
  let welcomeMessage = $state('');
  let originalName = $state('');
  let originalDescription = $state('');
  let originalMotd = $state('');
  let originalWelcomeMessage = $state('');
  let saveSuccess = $state(false);

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
    name !== originalName ||
      description !== originalDescription ||
      motd !== originalMotd ||
      welcomeMessage !== originalWelcomeMessage
  );

  // Query snapshots are external state. Reconcile each pristine field independently so a
  // background refresh cannot erase a draft in another field.
  $effect(() => {
    const next = snapshot;
    if (!next) return;
    untrack(() => {
      const nextDescription = next.description ?? '';
      const nextMotd = next.motd ?? '';
      const nextWelcomeMessage = next.welcomeMessage ?? '';
      if (name === originalName) name = next.name;
      if (description === originalDescription) description = nextDescription;
      if (motd === originalMotd) motd = nextMotd;
      if (welcomeMessage === originalWelcomeMessage) welcomeMessage = nextWelcomeMessage;
      originalName = next.name;
      originalDescription = nextDescription;
      originalMotd = nextMotd;
      originalWelcomeMessage = nextWelcomeMessage;
    });
  });

  $effect(() => {
    if (!snapshot || snapshot.viewerCanManageServer || !serverScope.isCurrent()) return;
    toast.error(m('server_settings.manage_denied'));
    goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverScope.serverId) }));
  });

  function isCurrentSession(
    variables: SettingsMutationScope | undefined
  ): variables is SettingsMutationScope {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  function mutationScope(): SettingsMutationScope {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      serverId,
      connection,
      queryKey: adminQueryKeys.serverSettings(serverId, connection),
      privacyGeneration
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

  const saveMutation = createMutation(
    () => ({
      mutationFn: ({ connection, input }: SaveVariables) =>
        updateServerConfig(connection.apiConfig, input),
      onSuccess: (profile, variables) => {
        if (!isCurrentSession(variables)) return;
        queryClient.setQueryData<AuthenticatedServerState>(variables.queryKey, (current) =>
          mergeEditableProfile(current, profile)
        );

        const nextDescription = profile.description ?? '';
        const nextMotd = profile.motd ?? '';
        const nextWelcomeMessage = profile.welcomeMessage ?? '';
        if (name.trim() === variables.input.name) name = profile.name;
        if (description.trim() === variables.input.description) description = nextDescription;
        if (motd === variables.input.motd) motd = nextMotd;
        if (welcomeMessage === variables.input.welcomeMessage) {
          welcomeMessage = nextWelcomeMessage;
        }
        originalName = profile.name;
        originalDescription = nextDescription;
        originalMotd = nextMotd;
        originalWelcomeMessage = nextWelcomeMessage;
        saveSuccess = true;
        const completedGeneration = privacyGeneration;
        setTimeout(() => {
          if (completedGeneration === privacyGeneration) saveSuccess = false;
        }, 3000);
      }
    }),
    () => queryClient
  );

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

  const assetMutation = createMutation(
    () => ({
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
        if (!isCurrentSession(variables)) return;
        updateAssetSnapshot(variables, profile);
        toast.success(assetSuccessMessage(variables.operation));
      },
      onError: (mutationError, variables) => {
        if (!isCurrentSession(variables)) return;
        toast.error(
          mutationError instanceof Error
            ? mutationError.message
            : assetErrorMessage(variables.operation)
        );
      },
      onSettled: (_profile, _error, variables) => {
        if (!isCurrentSession(variables)) return;
        if (variables.operation === 'upload-logo' && logoFileInput) logoFileInput.value = '';
        if (variables.operation === 'upload-banner' && bannerFileInput) bannerFileInput.value = '';
      }
    }),
    () => queryClient
  );

  // Keep the form serialized even if a privacy generation fences the pending result.
  const saving = $derived(saveMutation.isPending);
  const uploadingLogo = $derived(
    assetMutation.isPending &&
      isCurrentSession(assetMutation.variables) &&
      assetMutation.variables.operation === 'upload-logo'
  );
  const deletingLogo = $derived(
    assetMutation.isPending &&
      isCurrentSession(assetMutation.variables) &&
      assetMutation.variables.operation === 'delete-logo'
  );
  const uploadingBanner = $derived(
    assetMutation.isPending &&
      isCurrentSession(assetMutation.variables) &&
      assetMutation.variables.operation === 'upload-banner'
  );
  const deletingBanner = $derived(
    assetMutation.isPending &&
      isCurrentSession(assetMutation.variables) &&
      assetMutation.variables.operation === 'delete-banner'
  );
  const error = $derived.by(() => {
    if (settingsQuery.error) {
      return settingsQuery.error instanceof Error
        ? settingsQuery.error.message
        : m('server_settings.load_failed');
    }
    if (saveMutation.isError && isCurrentSession(saveMutation.variables)) {
      return saveMutation.error instanceof Error
        ? saveMutation.error.message
        : m('server_settings.save_failed');
    }
    return null;
  });

  function handleSave(e: Event) {
    e.preventDefault();
    if (nameError || !changed || saving) return;
    saveSuccess = false;
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

  function uploadLogoFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m('server_settings.invalid_image'));
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m('server_settings.image_too_large'));
      return;
    }

    if (!assetMutation.isPending) {
      assetMutation.mutate({ ...mutationScope(), operation: 'upload-logo', file });
    }
  }

  function handleLogoUpload(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) uploadLogoFile(file);
  }

  const logoDropZone = dropZone({
    onDrop: (files) => uploadLogoFile(files[0]),
    onDragStateChange: (dragging) => (isDraggingLogo = dragging),
    acceptedTypes: ['image/*']
  });

  function handleLogoDelete() {
    if (!logoUrl || assetMutation.isPending) return;
    assetMutation.mutate({ ...mutationScope(), operation: 'delete-logo' });
  }

  function uploadBannerFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m('server_settings.invalid_image'));
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m('server_settings.image_too_large'));
      return;
    }

    if (!assetMutation.isPending) {
      assetMutation.mutate({ ...mutationScope(), operation: 'upload-banner', file });
    }
  }

  function handleBannerUpload(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) uploadBannerFile(file);
  }

  const bannerDropZone = dropZone({
    onDrop: (files) => uploadBannerFile(files[0]),
    onDragStateChange: (dragging) => (isDraggingBanner = dragging),
    acceptedTypes: ['image/*']
  });

  function handleBannerDelete() {
    if (!bannerUrl || assetMutation.isPending) return;
    assetMutation.mutate({ ...mutationScope(), operation: 'delete-banner' });
  }
</script>

{#if loading}
  <div class="text-muted">{m('server_settings.loading')}</div>
{:else if loaded}
  <div class="flex flex-col gap-6">
    <!-- Server Details Form -->
    <Panel title={m('server_settings.general')} icon="iconify icon-[uil--edit]">
      <form onsubmit={handleSave} class="flex flex-col gap-4">
        <TextInput
          id="name"
          label={m('server_settings.name_label')}
          bind:value={name}
          required
          disabled={saving}
          error={nameError}
        />

        <TextArea
          id="description"
          label={m('server_settings.description_label')}
          bind:value={description}
          maxBytes={MAX_SERVER_DESCRIPTION_BYTES}
          disabled={saving}
          rows={2}
          description={m('server_settings.description_help')}
        />

        <TextInput
          id="motd"
          label={m('server_settings.motd_label')}
          bind:value={motd}
          disabled={saving}
          description={m('server_settings.motd_help')}
        />

        <TextArea
          id="welcome-message"
          label={m('server_settings.welcome_message_label')}
          bind:value={welcomeMessage}
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
            <span class="iconify icon-[uil--check]"></span>
            {m('server_settings.save_button')}
          </Button>
          {#if saveSuccess}
            <span class="text-sm text-success">{m('common.saved')}</span>
          {/if}
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
          class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface text-5xl font-black text-muted shadow-md"
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
              onchange={handleLogoUpload}
            />
            <Button
              variant="secondary"
              onclick={() => logoFileInput?.click()}
              loading={uploadingLogo}
              disabled={assetMutation.isPending}
              loadingText={m('server_settings.uploading')}
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify icon-[uil--image-upload]"></span>
                {logoUrl ? m('server_settings.logo_change') : m('server_settings.logo_upload')}
              </span>
            </Button>
            {#if logoUrl}
              <Button
                variant="ghost"
                onclick={handleLogoDelete}
                loading={deletingLogo}
                disabled={assetMutation.isPending}
                loadingText={m('server_settings.removing')}
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify icon-[uil--trash-alt]"></span>
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
          <div class="w-full max-w-md overflow-hidden rounded-lg bg-surface-emphasized shadow-md">
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
              onchange={handleBannerUpload}
            />
            <Button
              variant="secondary"
              onclick={() => bannerFileInput?.click()}
              loading={uploadingBanner}
              disabled={assetMutation.isPending}
              loadingText={m('server_settings.uploading')}
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify icon-[uil--image-upload]"></span>
                {bannerUrl
                  ? m('server_settings.banner_change')
                  : m('server_settings.banner_upload')}
              </span>
            </Button>
            {#if bannerUrl}
              <Button
                variant="ghost"
                onclick={handleBannerDelete}
                loading={deletingBanner}
                disabled={assetMutation.isPending}
                loadingText={m('server_settings.removing')}
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify icon-[uil--trash-alt]"></span>
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
  <div class="text-danger">{error}</div>
{/if}
