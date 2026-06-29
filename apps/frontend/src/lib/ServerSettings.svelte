<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import {
    deleteServerBanner,
    deleteServerLogo,
    getAuthenticatedServerState,
    updateServerConfig,
    uploadServerBanner,
    uploadServerLogo,
    type ServerStateAPIConfig
  } from '@chatto/api-client/serverState';
  import * as m from '$lib/i18n/messages';

  import { Panel } from '$lib/components/admin';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  const connection = useConnection();

  function apiConfig(): ServerStateAPIConfig {
    const currentConnection = connection();
    return {
      baseUrl: currentConnection.connectBaseUrl,
      bearerToken: currentConnection.bearerToken
    };
  }

  let loading = $state(true);
  let canManage = $state(false);
  let loaded = $state(false);
  let error = $state<string | null>(null);

  // Form state
  let name = $state('');
  let description = $state('');
  let motd = $state('');
  let welcomeMessage = $state('');
  let saving = $state(false);
  let saveSuccess = $state(false);

  // Logo state
  let logoUrl = $state<string | null>(null);
  let uploadingLogo = $state(false);
  let deletingLogo = $state(false);
  let logoFileInput = $state<HTMLInputElement>();

  // Banner state
  let bannerUrl = $state<string | null>(null);
  let uploadingBanner = $state(false);
  let deletingBanner = $state(false);
  let bannerFileInput = $state<HTMLInputElement>();

  // Drag state
  let isDraggingLogo = $state(false);
  let isDraggingBanner = $state(false);

  // Validation
  let nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m['server_settings.name_empty']();
    if (name !== name.trim()) return m['server_settings.name_trim']();
    return undefined;
  });

  // Load instance data and check permissions
  async function loadData() {
    loading = true;
    error = null;

    try {
      const state = await getAuthenticatedServerState(apiConfig());

      canManage = state.viewerCanManageServer;
      if (!canManage) {
        toast.error(m['server_settings.manage_denied']());
        goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(getActiveServer()) }));
        return;
      }

      loaded = true;
      name = state.name;
      description = state.description ?? '';
      motd = state.motd ?? '';
      welcomeMessage = state.welcomeMessage ?? '';
      logoUrl = state.logoUrl ?? null;
      bannerUrl = state.bannerUrl ?? null;
    } catch (_e) {
      error = m['server_settings.load_failed']();
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    loadData();
  });

  async function handleSave(e: Event) {
    e.preventDefault();

    if (nameError) return;

    saving = true;
    saveSuccess = false;
    error = null;

    try {
      const profile = await updateServerConfig(
        apiConfig(),
        {
          name: name.trim(),
          description: description.trim(),
          motd,
          welcomeMessage
        }
      );

      name = profile.name;
      description = profile.description ?? '';
      motd = profile.motd ?? '';
      welcomeMessage = profile.welcomeMessage ?? '';
      saveSuccess = true;
      setTimeout(() => (saveSuccess = false), 3000);
    } catch (_e) {
      error = m['server_settings.save_failed']();
    } finally {
      saving = false;
    }
  }

  async function uploadLogoFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m['server_settings.invalid_image']());
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m['server_settings.image_too_large']());
      return;
    }

    uploadingLogo = true;

    try {
      const profile = await uploadServerLogo(apiConfig(), file);
      logoUrl = profile.logoUrl ?? null;
      toast.success(m['server_settings.logo_uploaded']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['server_settings.logo_upload_failed']());
    } finally {
      uploadingLogo = false;
      if (logoFileInput) logoFileInput.value = '';
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

  async function handleLogoDelete() {
    if (!logoUrl) return;

    deletingLogo = true;

    try {
      const profile = await deleteServerLogo(apiConfig());
      logoUrl = profile.logoUrl ?? null;
      toast.success(m['server_settings.logo_removed']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['server_settings.logo_delete_failed']());
    } finally {
      deletingLogo = false;
    }
  }

  async function uploadBannerFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m['server_settings.invalid_image']());
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m['server_settings.image_too_large']());
      return;
    }

    uploadingBanner = true;

    try {
      const profile = await uploadServerBanner(apiConfig(), file);
      bannerUrl = profile.bannerUrl ?? null;
      toast.success(m['server_settings.banner_uploaded']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['server_settings.banner_upload_failed']());
    } finally {
      uploadingBanner = false;
      if (bannerFileInput) bannerFileInput.value = '';
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

  async function handleBannerDelete() {
    if (!bannerUrl) return;

    deletingBanner = true;

    try {
      const profile = await deleteServerBanner(apiConfig());
      bannerUrl = profile.bannerUrl ?? null;
      toast.success(m['server_settings.banner_removed']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['server_settings.banner_delete_failed']());
    } finally {
      deletingBanner = false;
    }
  }
</script>

{#if loading}
  <div class="text-muted">{m['server_settings.loading']()}</div>
{:else if error}
  <div class="text-danger">{error}</div>
{:else if loaded}
  <div class="flex flex-col gap-6">
    <!-- Server Details Form -->
    <Panel title={m['server_settings.general']()} icon="iconify uil--edit">
      <form onsubmit={handleSave} class="flex flex-col gap-4">
        <TextInput
          id="name"
          label={m['server_settings.name_label']()}
          bind:value={name}
          required
          disabled={saving}
          error={nameError}
        />

        <TextArea
          id="description"
          label={m['server_settings.description_label']()}
          bind:value={description}
          disabled={saving}
          rows={2}
          description={m['server_settings.description_help']()}
        />

        <TextInput
          id="motd"
          label={m['server_settings.motd_label']()}
          bind:value={motd}
          disabled={saving}
          description={m['server_settings.motd_help']()}
        />

        <TextArea
          id="welcome-message"
          label={m['server_settings.welcome_message_label']()}
          bind:value={welcomeMessage}
          rows={3}
          disabled={saving}
          description={m['server_settings.welcome_message_help']()}
        />

        <div class="flex items-center gap-3">
          <Button
            type="submit"
            loading={saving}
            disabled={!name.trim() || !!nameError}
            loadingText={m['server_settings.saving']()}
          >
            <span class="iconify uil--check"></span>
            {m['server_settings.save_button']()}
          </Button>
          {#if saveSuccess}
            <span class="text-sm text-green-600">{m['common.saved']()}</span>
          {/if}
        </div>
      </form>
    </Panel>

    <!-- Logo Section -->
    <Panel title={m['server_settings.logo']()} icon="iconify uil--image">
      <div
        class="relative flex items-start gap-6"
        data-testid="logo-drop-zone"
        {@attach logoDropZone}
      >
        <DropZoneOverlay
          visible={isDraggingLogo}
          title={m['server_settings.drop_image']()}
          subtitle={m['server_settings.logo_drop_subtitle']()}
        />
        <!-- Logo Preview -->
        <div
          class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface-200 text-5xl font-black text-muted shadow-md"
        >
          {#if logoUrl}
            <img
              src={logoUrl}
              alt={m['server_settings.logo_alt']()}
              class="h-full w-full object-cover"
            />
          {:else}
            {name?.[0]?.toUpperCase() || '?'}
          {/if}
        </div>

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            {m['server_settings.logo_description']()}
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
              loadingText={m['server_settings.uploading']()}
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify uil--image-upload"></span>
                {logoUrl ? m['server_settings.logo_change']() : m['server_settings.logo_upload']()}
              </span>
            </Button>
            {#if logoUrl}
              <Button
                variant="ghost"
                onclick={handleLogoDelete}
                loading={deletingLogo}
                loadingText={m['server_settings.removing']()}
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify uil--trash-alt"></span>
                  {m['server_settings.remove']()}
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>

    <!-- Banner Section -->
    <Panel title={m['server_settings.banner']()} icon="iconify uil--scenery">
      <div
        class="relative flex flex-col gap-4"
        data-testid="banner-drop-zone"
        {@attach bannerDropZone}
      >
        <DropZoneOverlay
          visible={isDraggingBanner}
          title={m['server_settings.drop_image']()}
          subtitle={m['server_settings.banner_drop_subtitle']()}
        />
        <!-- Banner Preview — capped width so the OG-aspect 1200×630 doesn't
             swallow the panel on wide layouts. -->
        {#if bannerUrl}
          <div class="w-full max-w-md overflow-hidden rounded-lg bg-surface-200 shadow-md">
            <img
              src={bannerUrl}
              alt={m['server_settings.banner_alt']()}
              class="aspect-[1200/630] w-full object-cover"
            />
          </div>
        {:else}
          <div
            class="flex aspect-[1200/630] w-full max-w-md items-center justify-center rounded-lg border-2 border-dashed border-border bg-surface-100 text-muted"
          >
            <span class="text-sm">{m['server_settings.no_banner']()}</span>
          </div>
        {/if}

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            {m['server_settings.banner_description']()}
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
              loadingText={m['server_settings.uploading']()}
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify uil--image-upload"></span>
                {bannerUrl
                  ? m['server_settings.banner_change']()
                  : m['server_settings.banner_upload']()}
              </span>
            </Button>
            {#if bannerUrl}
              <Button
                variant="ghost"
                onclick={handleBannerDelete}
                loading={deletingBanner}
                loadingText={m['server_settings.removing']()}
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify uil--trash-alt"></span>
                  {m['server_settings.remove']()}
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>
  </div>
{/if}
