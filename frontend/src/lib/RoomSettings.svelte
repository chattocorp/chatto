<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let { roomId }: { roomId: string } = $props();

  const RoomSettingsDataQuery = graphql(`
    query RoomSettingsData($roomId: ID!) {
      room(roomId: $roomId) {
        id
        name
        description
      }
      instance {
        viewerCanManageRooms
      }
    }
  `);

  const UpdateRoomSettingsMutation = graphql(`
    mutation UpdateRoomSettings($input: UpdateRoomInput!) {
      updateRoom(input: $input) {
        id
        name
        description
      }
    }
  `);

  // Form state - initialized from query via onCompleted
  let name = $state('');
  let description = $state('');
  let saveSuccess = $state(false);

  // Load room data - useQuery handles race conditions automatically
  const roomQuery = useQuery(RoomSettingsDataQuery, () => ({ roomId }), {
    onCompleted: (data) => {
      if (data.room) {
        name = data.room.name;
        description = data.room.description || '';
      }
    }
  });

  const updateMutation = useMutation(UpdateRoomSettingsMutation);

  let room = $derived(roomQuery.data?.room ?? null);
  let loading = $derived(roomQuery.loading);
  let error = $derived(roomQuery.error ?? updateMutation.error ?? null);
  let saving = $derived(updateMutation.loading);

  // Validation
  let nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return 'Room name cannot be empty';
    if (name !== name.trim()) return 'Room name cannot have leading or trailing whitespace';
    // Room names have stricter validation (URL-safe characters)
    if (!/^[a-zA-Z0-9_-]+$/.test(name.trim())) {
      return 'Room name can only contain letters, numbers, hyphens, and underscores';
    }
    if (name.length > 30) {
      return 'Room name cannot exceed 30 characters';
    }
    return undefined;
  });

  async function handleSave(e: Event) {
    e.preventDefault();

    // Validate before submission
    if (nameError) return;

    saveSuccess = false;

    const result = await updateMutation.execute({
      input: {
        roomId,
        name: name.trim(),
        description: description?.trim() || null
      }
    });

    if (result.data?.updateRoom) {
      saveSuccess = true;
      toast.success('Room settings saved');
      setTimeout(() => (saveSuccess = false), 3000);
    }
  }
</script>

{#if loading}
  <div class="text-muted">Loading...</div>
{:else if error}
  <div class="text-danger">{error}</div>
{:else if room}
  <div class="flex flex-col gap-6">
    <!-- Room Details Form -->
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
          placeholder="Optional description for this room"
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
  </div>
{/if}
