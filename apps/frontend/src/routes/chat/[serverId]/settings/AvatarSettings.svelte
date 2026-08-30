<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createUserAPI } from '$lib/api-client/users';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';

  // The server route keys its subtree by server, so the current-user store is
  // stable for the component lifetime while its fields remain reactive.
  const serverScope = useServerScope();
  const currentUser = serverScope.store.currentUser;

  async function uploadAvatar(file: File): Promise<boolean> {
    const userId = currentUser.user?.id;
    if (!userId) return false;
    const updated = await serverScope.connection.getAPI(createUserAPI).uploadAvatar(userId, file);
    if (!serverScope.isCurrent() || currentUser.user?.id !== userId) return false;
    currentUser.user = { ...currentUser.user, avatarUrl: updated.avatarUrl };
    return true;
  }

  async function deleteAvatar(): Promise<boolean> {
    const userId = currentUser.user?.id;
    if (!userId) return false;
    const updated = await serverScope.connection.getAPI(createUserAPI).deleteAvatar(userId);
    if (!serverScope.isCurrent() || currentUser.user?.id !== userId) return false;
    currentUser.user = { ...currentUser.user, avatarUrl: updated.avatarUrl };
    return true;
  }
</script>

{#if currentUser.user}
  <AvatarEditor user={currentUser.user} onupload={uploadAvatar} ondelete={deleteAvatar} />
{/if}
