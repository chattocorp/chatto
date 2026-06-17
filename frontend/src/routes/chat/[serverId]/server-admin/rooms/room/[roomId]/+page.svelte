<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const ROOM_INFORMATION_MAX_LENGTH = 10000;

  const roomId = $derived(page.params.roomId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const connection = useConnection();
  const stores = serverRegistry.getStore(getActiveServer());
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  const AdminRoomDetailQuery = graphql(`
    query AdminRoomDetail($roomId: ID!) {
      admin {
        room(roomId: $roomId) {
          id
          name
          description
          information
        }
      }
    }
  `);

  const AdminUpdateRoomDetailsMutation = graphql(`
    mutation AdminUpdateRoomDetails($input: UpdateRoomInput!) {
      updateRoom(input: $input) {
        id
        name
        description
      }
    }
  `);

  const AdminUpdateRoomInformationMutation = graphql(`
    mutation AdminUpdateRoomInformation($input: UpdateRoomInformationInput!) {
      updateRoomInformation(input: $input) {
        id
        information
      }
    }
  `);

  type RoomFormState = {
    id: string;
    name: string;
    description: string;
    information: string;
  };

  let original = $state<RoomFormState | null>(null);
  let name = $state('');
  let description = $state('');
  let information = $state('');
  let savingDetails = $state(false);
  let savingInformation = $state(false);

  function seedRoom(room: {
    id: string;
    name: string;
    description?: string | null;
    information?: string | null;
  }) {
    original = {
      id: room.id,
      name: room.name,
      description: room.description ?? '',
      information: room.information ?? ''
    };
    name = original.name;
    description = original.description;
    information = original.information;
  }

  const roomQuery = useQuery(AdminRoomDetailQuery, () => ({ roomId }), {
    onCompleted: (data) => {
      const room = data.admin?.room;
      if (room) seedRoom(room);
    },
    onError: (error) => toast.error(`Failed to load room: ${error}`)
  });

  const room = $derived(roomQuery.data?.admin?.room ?? null);
  const pageTitle = $derived(room ? `Room — #${room.name}` : 'Room');

  const nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return 'Room name cannot be empty';
    if (name !== name.trim()) return 'Room name cannot have leading or trailing whitespace';
    if (!/^[a-zA-Z0-9_-]+$/.test(name.trim())) {
      return 'Room name can only contain letters, numbers, hyphens, and underscores';
    }
    if (name.length > 30) return 'Room name cannot exceed 30 characters';
    return undefined;
  });

  const informationError = $derived(
    information.length > ROOM_INFORMATION_MAX_LENGTH
      ? `Room information cannot exceed ${ROOM_INFORMATION_MAX_LENGTH} characters`
      : undefined
  );
  const detailsDirty = $derived(
    !!original && (name !== original.name || description !== original.description)
  );
  const informationDirty = $derived(!!original && information !== original.information);

  async function saveDetails() {
    if (!original || savingDetails || nameError || !detailsDirty) return;
    savingDetails = true;
    try {
      const result = await connection()
        .client.mutation(AdminUpdateRoomDetailsMutation, {
          input: {
            roomId: original.id,
            name: name.trim(),
            description: description.trim() || null
          }
        })
        .toPromise();

      if (result.error || !result.data?.updateRoom) {
        toast.error('Failed to update room details');
        return;
      }

      const updated = result.data.updateRoom;
      original = {
        ...original,
        name: updated.name,
        description: updated.description ?? ''
      };
      name = original.name;
      description = original.description;
      toast.success('Room details saved');
      void stores.adminRoomLayout.refresh();
    } finally {
      savingDetails = false;
    }
  }

  async function saveInformation() {
    if (!original || savingInformation || informationError || !informationDirty) return;
    savingInformation = true;
    try {
      const result = await connection()
        .client.mutation(AdminUpdateRoomInformationMutation, {
          input: {
            roomId: original.id,
            information
          }
        })
        .toPromise();

      if (result.error || !result.data?.updateRoomInformation) {
        toast.error('Failed to update room information');
        return;
      }

      const updated = result.data.updateRoomInformation;
      original = {
        ...original,
        information: updated.information ?? ''
      };
      information = original.information;
      toast.success('Room information saved');
    } finally {
      savingInformation = false;
    }
  }

  function handleInformationKeydown(event: KeyboardEvent) {
    if (!(event.metaKey || event.ctrlKey) || event.key !== 'Enter') return;
    if (!informationDirty || informationError || savingInformation) return;
    event.preventDefault();
    void saveInformation();
  }
</script>

<PageTitle title={`${pageTitle} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={room ? `#${room.name}` : ''}
    subtitle="Room details, information, and permission overrides"
    {backHref}
    backLabel="Back to rooms"
    showMobileNav
  />

  <div class="flex flex-col gap-8 overflow-y-auto p-6">
    {#if roomQuery.loading && !original}
      <div class="text-muted">Loading room...</div>
    {:else if roomQuery.error}
      <Hint tone="danger">{roomQuery.error}</Hint>
    {:else if !room}
      <Hint tone="danger">Room not found or not manageable.</Hint>
    {:else}
      <section id="details" class="flex max-w-3xl flex-col gap-4">
        <div>
          <h2 class="text-lg font-semibold">Details</h2>
        </div>
        <TextInput
          id="room-name"
          label="Name"
          bind:value={name}
          required
          maxlength={30}
          disabled={savingDetails}
          error={nameError}
        />
        <TextArea
          id="room-description"
          label="Description"
          bind:value={description}
          rows={3}
          disabled={savingDetails}
          placeholder="Optional description for this room"
        />
        <div>
          <Button
            onclick={saveDetails}
            disabled={!detailsDirty || !!nameError || savingDetails}
            loading={savingDetails}
            loadingText="Saving..."
          >
            Save Details
          </Button>
        </div>
      </section>

      <section id="information" class="flex max-w-3xl flex-col gap-4">
        <div>
          <h2 class="text-lg font-semibold">Room Information</h2>
        </div>
        <TextArea
          id="room-information"
          label="Markdown"
          bind:value={information}
          rows={12}
          maxlength={ROOM_INFORMATION_MAX_LENGTH}
          disabled={savingInformation}
          error={informationError}
          placeholder="Add Markdown-formatted room information..."
          testid="room-information-editor"
          onkeydown={handleInformationKeydown}
        />
        <div class="flex items-center justify-between gap-3">
          <p class={['text-sm', informationError ? 'text-danger' : 'text-muted']}>
            {information.length}/{ROOM_INFORMATION_MAX_LENGTH}
          </p>
          <Button
            onclick={saveInformation}
            disabled={!informationDirty || !!informationError || savingInformation}
            loading={savingInformation}
            loadingText="Saving..."
          >
            Save Room Information
          </Button>
        </div>
      </section>

      <section id="permissions" class="flex flex-col gap-4">
        <div>
          <h2 class="text-lg font-semibold">Permissions</h2>
        </div>
        <Hint>
          Per-room overrides for this room. Values set here take precedence over the group's
          and the server-wide defaults.
        </Hint>
        <PermissionMatrix {roomId} />
      </section>
    {/if}
  </div>
</div>
