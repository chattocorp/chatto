<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';

  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  const connection = useConnection();

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
    if (name.trim() === '') return 'Name cannot be empty';
    if (name !== name.trim()) return 'Name cannot have leading or trailing whitespace';
    return undefined;
  });

  // Load instance data and check permissions
  async function loadData() {
    loading = true;
    error = null;

    try {
      const result = await connection().client
        .query(
          graphql(`
            query ServerSettingsModal {
              server {
                config {
                  serverName
                  description
                  motd
                  welcomeMessage
                  logoUrl
                  bannerUrl
                }
                viewerCanManageInstance
              }
            }
          `),
          {}
        )
        .toPromise();

      if (result.error) {
        error = 'Failed to load instance';
        return;
      }

      if (!result.data?.server) {
        error = 'Server not found';
        return;
      }

      canManage = result.data.server.viewerCanManageInstance;
      if (!canManage) {
        toast.error('You do not have permission to manage this instance');
        goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(getActiveServer()) }));
        return;
      }

      loaded = true;
      name = result.data.server.config.serverName;
      description = result.data.server.config.description ?? '';
      motd = result.data.server.config.motd ?? '';
      welcomeMessage = result.data.server.config.welcomeMessage ?? '';
      logoUrl = result.data.server.config.logoUrl ?? null;
      bannerUrl = result.data.server.config.bannerUrl ?? null;
    } catch (_e) {
      error = 'Failed to load instance';
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
      const result = await connection().client
        .mutation(
          graphql(`
            mutation UpdateServerSettingsModal($input: UpdateServerInput!) {
              updateServer(input: $input) {
                config {
                  serverName
                  description
                  motd
                  welcomeMessage
                }
              }
            }
          `),
          {
            input: {
              name: name.trim(),
              description: description.trim(),
              motd,
              welcomeMessage
            }
          }
        )
        .toPromise();

      if (result.error) {
        error = 'Failed to save changes';
        return;
      }

      if (result.data?.updateServer) {
        saveSuccess = true;
        setTimeout(() => (saveSuccess = false), 3000);
      }
    } catch (_e) {
      error = 'Failed to save changes';
    } finally {
      saving = false;
    }
  }

  async function uploadLogoFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error('Please select an image file');
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error('Image must be less than 10MB');
      return;
    }

    uploadingLogo = true;

    try {
      const result = await connection().client
        .mutation(
          graphql(`
            mutation UploadInstanceLogo($input: UploadServerLogoInput!) {
              uploadServerLogo(input: $input) {
                config {
                  logoUrl
                }
              }
            }
          `),
          { input: { file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      logoUrl = result.data?.uploadServerLogo.config.logoUrl ?? null;
      toast.success('Logo uploaded successfully');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to upload logo');
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
      const result = await connection().client
        .mutation(
          graphql(`
            mutation DeleteInstanceLogo {
              deleteServerLogo {
                config {
                  logoUrl
                }
              }
            }
          `),
          {}
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      logoUrl = null;
      toast.success('Logo removed');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete logo');
    } finally {
      deletingLogo = false;
    }
  }

  async function uploadBannerFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error('Please select an image file');
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error('Image must be less than 10MB');
      return;
    }

    uploadingBanner = true;

    try {
      const result = await connection().client
        .mutation(
          graphql(`
            mutation UploadInstanceBanner($input: UploadServerBannerInput!) {
              uploadServerBanner(input: $input) {
                config {
                  bannerUrl
                }
              }
            }
          `),
          { input: { file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      bannerUrl = result.data?.uploadServerBanner.config.bannerUrl ?? null;
      toast.success('Banner uploaded successfully');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to upload banner');
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
      const result = await connection().client
        .mutation(
          graphql(`
            mutation DeleteInstanceBanner {
              deleteServerBanner {
                config {
                  bannerUrl
                }
              }
            }
          `),
          {}
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      bannerUrl = null;
      toast.success('Banner removed');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete banner');
    } finally {
      deletingBanner = false;
    }
  }
</script>

{#if loading}
  <div class="text-muted">Loading...</div>
{:else if error}
  <div class="text-danger">{error}</div>
{:else if loaded}
  <div class="flex flex-col gap-6">
    <!-- Server Details Form -->
    <Panel title="General" icon="iconify uil--edit">
      <form onsubmit={handleSave} class="flex flex-col gap-4">
        <TextInput
          id="name"
          label="Name"
          bind:value={name}
          required
          disabled={saving}
          error={nameError}
        />

        <TextArea
          id="description"
          label="Description"
          bind:value={description}
          disabled={saving}
          rows={2}
          description="Shown on the welcome screen and used as the description for link previews."
        />

        <TextInput
          id="motd"
          label="Message of the Day"
          bind:value={motd}
          disabled={saving}
          description="Single-line message displayed in the chat header. Supports markdown."
        />

        <TextArea
          id="welcome-message"
          label="Welcome Message"
          bind:value={welcomeMessage}
          rows={3}
          disabled={saving}
          description="Shown on the login page. Supports markdown."
        />

        <div class="flex items-center gap-3">
          <Button
            type="submit"
            loading={saving}
            disabled={!name.trim() || !!nameError}
            loadingText="Saving..."
          >
            <span class="iconify uil--check"></span>
            Save Changes
          </Button>
          {#if saveSuccess}
            <span class="text-sm text-green-600">Saved!</span>
          {/if}
        </div>
      </form>
    </Panel>

    <!-- Logo Section -->
    <Panel title="Logo" icon="iconify uil--image">
      <div class="relative flex items-start gap-6" data-testid="logo-drop-zone" {@attach logoDropZone}>
        <DropZoneOverlay visible={isDraggingLogo} title="Drop image" subtitle="Upload as instance logo" />
        <!-- Logo Preview -->
        <div
          class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface-200 text-5xl font-black text-muted shadow-md"
        >
          {#if logoUrl}
            <img src={logoUrl} alt="Server logo" class="h-full w-full object-cover" />
          {:else}
            {name?.[0]?.toUpperCase() || '?'}
          {/if}
        </div>

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            Upload a logo for your instance. Images will be resized to 512×512 pixels.
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
              loadingText="Uploading..."
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify uil--image-upload"></span>
                {logoUrl ? 'Change Logo' : 'Upload Logo'}
              </span>
            </Button>
            {#if logoUrl}
              <Button
                variant="ghost"
                onclick={handleLogoDelete}
                loading={deletingLogo}
                loadingText="Removing..."
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify uil--trash-alt"></span>
                  Remove
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>

    <!-- Banner Section -->
    <Panel title="Banner" icon="iconify uil--scenery">
      <div class="relative flex flex-col gap-4" data-testid="banner-drop-zone" {@attach bannerDropZone}>
        <DropZoneOverlay visible={isDraggingBanner} title="Drop image" subtitle="Upload as instance banner" />
        <!-- Banner Preview — capped width so the OG-aspect 1200×630 doesn't
             swallow the panel on wide layouts. -->
        {#if bannerUrl}
          <div class="w-full max-w-md overflow-hidden rounded-lg bg-surface-200 shadow-md">
            <img src={bannerUrl} alt="Server banner" class="aspect-[1200/630] w-full object-cover" />
          </div>
        {:else}
          <div
            class="flex aspect-[1200/630] w-full max-w-md items-center justify-center rounded-lg border-2 border-dashed border-border bg-surface-100 text-muted"
          >
            <span class="text-sm">No banner set</span>
          </div>
        {/if}

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            Upload a banner for your instance. The banner doubles as the link-preview image — images
            are resized to 1200×630 pixels (the standard OpenGraph aspect ratio).
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
              loadingText="Uploading..."
            >
              <span class="inline-flex items-center gap-2">
                <span class="iconify uil--image-upload"></span>
                {bannerUrl ? 'Change Banner' : 'Upload Banner'}
              </span>
            </Button>
            {#if bannerUrl}
              <Button
                variant="ghost"
                onclick={handleBannerDelete}
                loading={deletingBanner}
                loadingText="Removing..."
              >
                <span class="inline-flex items-center gap-2 text-error">
                  <span class="iconify uil--trash-alt"></span>
                  Remove
                </span>
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Panel>
  </div>
{/if}
