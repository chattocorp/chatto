<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createUserAPI } from '@chatto/client/api/users';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';

  // The server route keys its subtree by server, so the current-user store is
  // stable for the component lifetime while its fields remain reactive.
  const serverScope = useServerScope();
  const currentUser = serverScope.store.currentUser;

  async function uploadAvatar(file: File): Promise<boolean> {
    const userId = serverScope.store.accountId;
    if (!userId) return false;
    const updated = await serverScope.connection.getAPI(createUserAPI).uploadAvatar(userId, file);
    if (!serverScope.isCurrent()) return false;
    return currentUser.update(userId, () => ({ avatarUrl: updated.avatarUrl }));
  }

  async function deleteAvatar(): Promise<boolean> {
    const userId = serverScope.store.accountId;
    if (!userId) return false;
    const updated = await serverScope.connection.getAPI(createUserAPI).deleteAvatar(userId);
    if (!serverScope.isCurrent()) return false;
    return currentUser.update(userId, () => ({ avatarUrl: updated.avatarUrl }));
  }
</script>

{#if currentUser.user}
  <AvatarEditor user={currentUser.user} onupload={uploadAvatar} ondelete={deleteAvatar} />
{/if}
