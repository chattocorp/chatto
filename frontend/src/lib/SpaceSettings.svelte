<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';

  const getInstanceId = getActiveInstance();
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  let { spaceId }: { spaceId: string } = $props();

  const connection = useConnection();

  let loading = $state(true);
  let canManage = $state(false);
  let space = $state<{
    id: string;
    name: string;
    description?: string | null;
    logoUrl?: string | null;
    bannerUrl?: string | null;
  } | null>(null);
  let error = $state<string | null>(null);

  // Form state
  let name = $state('');
  let description = $state('');
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
    if (name.trim() === '') return 'Space name cannot be empty';
    if (name !== name.trim()) return 'Space name cannot have leading or trailing whitespace';
    return undefined;
  });

  // Load space data and check permissions
  async function loadData() {
    loading = true;
    error = null;

    try {
      const result = await connection().client
        .query(
          graphql(`
            query SpaceSettingsModal($spaceId: ID!) {
              space(id: $spaceId) {
                id
                name
                description
                logoUrl
                bannerUrl
                viewerCanManageSpace
              }
            }
          `),
          { spaceId }
        )
        .toPromise();

      if (result.error) {
        error = 'Failed to load space';
        return;
      }

      if (!result.data?.space) {
        error = 'Space not found';
        return;
      }

      canManage = result.data.space.viewerCanManageSpace;
      if (!canManage) {
        toast.error('You do not have permission to manage this space');
        goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(getInstanceId()) }));
        return;
      }

      space = result.data.space;
      name = space.name;
      description = space.description || '';
      logoUrl = space.logoUrl || null;
      bannerUrl = space.bannerUrl || null;
    } catch (_e) {
      error = 'Failed to load space';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    loadData();
  });

  async function handleSave(e: Event) {
    e.preventDefault();

    // Validate before submission
    if (nameError) return;

    saving = true;
    saveSuccess = false;
    error = null;

    try {
      const result = await connection().client
        .mutation(
          graphql(`
            mutation UpdateSpaceSettingsModal($input: UpdateSpaceInput!) {
              updateSpace(input: $input) {
                id
                name
                description
              }
            }
          `),
          { input: { id: spaceId, name: name.trim(), description: description?.trim() || null } }
        )
        .toPromise();

      if (result.error) {
        error = 'Failed to save changes';
        return;
      }

      if (result.data?.updateSpace) {
        space = result.data.updateSpace;
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
            mutation UploadSpaceLogo($input: UploadSpaceLogoInput!) {
              uploadSpaceLogo(input: $input) {
                id
                logoUrl
              }
            }
          `),
          { input: { spaceId, file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      logoUrl = result.data?.uploadSpaceLogo.logoUrl ?? null;
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
            mutation DeleteSpaceLogo($input: DeleteSpaceLogoInput!) {
              deleteSpaceLogo(input: $input) {
                id
                logoUrl
              }
            }
          `),
          { input: { spaceId } }
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
            mutation UploadSpaceBanner($input: UploadSpaceBannerInput!) {
              uploadSpaceBanner(input: $input) {
                id
                bannerUrl
              }
            }
          `),
          { input: { spaceId, file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      bannerUrl = result.data?.uploadSpaceBanner.bannerUrl ?? null;
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
            mutation DeleteSpaceBanner($input: DeleteSpaceBannerInput!) {
              deleteSpaceBanner(input: $input) {
                id
                bannerUrl
              }
            }
          `),
          { input: { spaceId } }
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
{:else if space}
  <div class="flex flex-col gap-6">
    <!-- Space Details Form -->
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
          rows={3}
          disabled={saving}
          placeholder="Optional description for this space"
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
        <DropZoneOverlay visible={isDraggingLogo} title="Drop image" subtitle="Upload as space logo" />
        <!-- Logo Preview -->
        <div
          class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl bg-surface-200 text-5xl font-black text-muted shadow-md"
        >
          {#if logoUrl}
            <img src={logoUrl} alt="Space logo" class="h-full w-full object-cover" />
          {:else}
            {name?.[0]?.toUpperCase() || '?'}
          {/if}
        </div>

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            Upload a logo for your space. Images will be resized to 512×512 pixels.
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
        <DropZoneOverlay visible={isDraggingBanner} title="Drop image" subtitle="Upload as space banner" />
        <!-- Banner Preview -->
        {#if bannerUrl}
          <div class="w-full overflow-hidden rounded-lg bg-surface-200 shadow-md">
            <img src={bannerUrl} alt="Space banner" class="aspect-[4/3] w-full object-cover" />
          </div>
        {:else}
          <div
            class="flex aspect-[4/3] w-full items-center justify-center rounded-lg border-2 border-dashed border-border bg-surface-100 text-muted"
          >
            <span class="text-sm">No banner set</span>
          </div>
        {/if}

        <!-- Upload Controls -->
        <div class="flex flex-col gap-3">
          <p class="text-sm text-muted">
            Upload a banner for your space. Images will be resized to 768×576 pixels (4:3 aspect
            ratio).
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
