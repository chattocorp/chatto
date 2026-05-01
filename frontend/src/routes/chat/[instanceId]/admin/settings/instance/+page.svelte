<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import FormSection from '$lib/ui/FormSection.svelte';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  let instanceName = $state('');
  let ogTitle = $state('');
  let ogDescription = $state('');
  let ogImageUrl = $state<string | null>(null);
  let motd = $state('');
  let welcomeMessage = $state('');
  let blockedUsernames = $state('');
  let isConfigured = $state(false);

  // OG Image upload state
  let ogImageFileInput = $state<HTMLInputElement>();
  let isDraggingOGImage = $state(false);

  const defaultBlockedUsernames = 'root\nadmin\nsuperuser\nop\noperator\nsupport';

  function applyConfig(cfg: {
    isConfigured: boolean;
    instanceName: string;
    ogTitle?: string | null;
    ogDescription?: string | null;
    ogImageUrl?: string | null;
    motd?: string | null;
    welcomeMessage?: string | null;
    blockedUsernames?: string | null;
  }) {
    instanceName = cfg.instanceName;
    ogTitle = cfg.ogTitle ?? '';
    ogDescription = cfg.ogDescription ?? '';
    if ('ogImageUrl' in cfg) ogImageUrl = cfg.ogImageUrl ?? null;
    motd = cfg.motd ?? '';
    welcomeMessage = cfg.welcomeMessage ?? '';
    blockedUsernames = cfg.blockedUsernames ?? defaultBlockedUsernames;
    isConfigured = cfg.isConfigured;
  }

  // Load config
  const configQuery = useQuery(
    graphql(`
      query AdminInstanceConfig {
        admin {
          instanceConfig {
            isConfigured
            instanceName
            ogTitle
            ogDescription
            ogImageUrl
            motd
            welcomeMessage
            blockedUsernames
          }
        }
      }
    `),
    () => ({}),
    {
      onCompleted: (data) => {
        if (data.admin?.instanceConfig) {
          applyConfig(data.admin.instanceConfig);
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  // Save config mutation
  const saveMutation = useMutation(
    graphql(`
      mutation UpdateInstanceConfig($input: UpdateInstanceConfigInput!) {
        admin {
          updateInstanceConfig(input: $input) {
            isConfigured
            instanceName
            ogTitle
            ogDescription
            motd
            welcomeMessage
            blockedUsernames
          }
        }
      }
    `),
    {
      onCompleted: (data) => {
        if (data.admin?.updateInstanceConfig) {
          applyConfig(data.admin.updateInstanceConfig);
          toast.success('Settings saved');
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  // Reset config mutation
  const resetMutation = useMutation(
    graphql(`
      mutation ResetInstanceConfig {
        admin {
          resetInstanceConfig
        }
      }
    `),
    {
      onCompleted: () => {
        instanceName = 'Chatto';
        ogTitle = '';
        ogDescription = '';
        ogImageUrl = null;
        motd = '';
        welcomeMessage = '';
        blockedUsernames = defaultBlockedUsernames;
        isConfigured = false;
        toast.success('Configuration reset to defaults');
      },
      onError: (err) => toast.error(err)
    }
  );

  // OG Image upload mutation
  const uploadImageMutation = useMutation(
    graphql(`
      mutation UploadInstanceOGImage($input: UploadInstanceOGImageInput!) {
        admin {
          uploadInstanceOGImage(input: $input) {
            ogImageUrl
          }
        }
      }
    `),
    {
      onCompleted: (data) => {
        ogImageUrl = data.admin?.uploadInstanceOGImage.ogImageUrl ?? null;
        toast.success('Image uploaded successfully');
      },
      onError: (err) => toast.error(err)
    }
  );

  // OG Image delete mutation
  const deleteImageMutation = useMutation(
    graphql(`
      mutation DeleteInstanceOGImage {
        admin {
          deleteInstanceOGImage {
            ogImageUrl
          }
        }
      }
    `),
    {
      onCompleted: () => {
        ogImageUrl = null;
        toast.success('Image removed');
      },
      onError: (err) => toast.error(err)
    }
  );

  const saving = $derived(saveMutation.loading || resetMutation.loading);

  async function saveConfig() {
    await saveMutation.execute({
      input: { instanceName, ogTitle, ogDescription, motd, welcomeMessage, blockedUsernames }
    });
  }

  async function uploadOGImageFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error('Please select an image file');
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error('Image must be less than 10MB');
      return;
    }

    await uploadImageMutation.execute({ input: { file } });

    if (ogImageFileInput) ogImageFileInput.value = '';
  }

  function handleOGImageUpload(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) uploadOGImageFile(file);
  }

  const ogImageDropZone = dropZone({
    onDrop: (files) => uploadOGImageFile(files[0]),
    onDragStateChange: (dragging) => (isDraggingOGImage = dragging),
    acceptedTypes: ['image/*']
  });

  async function handleOGImageDelete() {
    if (!ogImageUrl) return;
    await deleteImageMutation.execute({});
  }
</script>

<PageTitle title="Instance Settings | Admin" />

<PaneHeader
  title="Instance Settings"
  subtitle="Configure instance name, branding, and messages"
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if configQuery.loading}
    <div class="text-muted">Loading...</div>
  {:else}
    <form
      onsubmit={(e) => {
        e.preventDefault();
        saveConfig();
      }}
      class="flex max-w-xl flex-col gap-6"
    >
      <FormSection title="Branding">
        <div class="flex flex-col gap-4">
          <TextInput
            label="Instance Name"
            id="instance-name"
            bind:value={instanceName}
            disabled={saving}
            description="Displayed in page titles. Defaults to 'Chatto' if empty."
          />
        </div>
      </FormSection>

      <FormSection title="Link Previews" bordered>
        <div class="flex flex-col gap-4">
          <TextInput
            label="Title"
            id="og-title"
            bind:value={ogTitle}
            disabled={saving}
            description="Title shown in link previews. Defaults to Instance Name if empty."
          />

          <TextArea
            label="Description"
            id="og-description"
            bind:value={ogDescription}
            rows={2}
            disabled={saving}
            description="Description shown when sharing links to this instance."
          />

          <!-- OG Image Upload -->
          <div class="relative flex flex-col gap-2" {@attach ogImageDropZone}>
            <DropZoneOverlay visible={isDraggingOGImage} title="Drop image" subtitle="Upload as preview image" />
            <label class="text-sm font-medium" for="og-image">Preview Image</label>
            <p class="text-sm text-muted">
              Recommended size: 1200×630 pixels (1.91:1 ratio). Image will be resized automatically.
            </p>

            {#if ogImageUrl}
              <div class="flex items-start gap-4">
                <img
                  src={ogImageUrl}
                  alt="Link preview"
                  class="h-auto w-64 rounded border border-border object-cover"
                />
                <Button
                  variant="ghost"
                  onclick={handleOGImageDelete}
                  loading={deleteImageMutation.loading}
                  loadingText="Removing..."
                >
                  <span class="inline-flex items-center gap-2 text-error">
                    <span class="iconify uil--trash-alt"></span>
                    Remove
                  </span>
                </Button>
              </div>
            {/if}

            <input
              type="file"
              id="og-image"
              accept="image/*"
              class="hidden"
              bind:this={ogImageFileInput}
              onchange={handleOGImageUpload}
            />
            <div>
              <Button
                variant="secondary"
                onclick={() => ogImageFileInput?.click()}
                loading={uploadImageMutation.loading}
                loadingText="Uploading..."
              >
                <span class="inline-flex items-center gap-2">
                  <span class="iconify uil--image-upload"></span>
                  {ogImageUrl ? 'Change Image' : 'Upload Image'}
                </span>
              </Button>
            </div>
          </div>
        </div>
      </FormSection>

      <FormSection title="Messages" bordered>
        <div class="flex flex-col gap-4">
          <TextInput
            label="Message of the Day"
            id="motd"
            bind:value={motd}
            disabled={saving}
            description="Single-line message displayed in the header bar."
          />

          <TextArea
            label="Welcome Message"
            id="welcome-message"
            bind:value={welcomeMessage}
            rows={3}
            disabled={saving}
            description="Shown on the login page. Supports markdown."
          />
        </div>
      </FormSection>

      <FormSection title="Security" bordered>
        <div class="flex flex-col gap-4">
          <TextArea
            label="Blocked Usernames"
            id="blocked-usernames"
            bind:value={blockedUsernames}
            rows={6}
            disabled={saving}
            description="One per line. Users cannot register with these names."
          />
        </div>
      </FormSection>

      <div class="flex items-center gap-4 border-t border-border pt-6">
        <Button type="submit" disabled={saving} loading={saving}>
          <span class="iconify uil--check"></span>
          Save
        </Button>

        {#if isConfigured}
          <Button
            variant="ghost"
            onclick={() => resetMutation.execute({})}
            disabled={saving}
          >
            <span class="iconify uil--redo"></span>
            Reset to Defaults
          </Button>
        {/if}
      </div>
    </form>
  {/if}
</div>
