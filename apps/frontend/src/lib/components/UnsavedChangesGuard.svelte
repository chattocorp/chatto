<!-- @component Protects unsaved settings during navigation; permission loss disables the guard. -->
<script lang="ts">
  import { beforeNavigate, goto } from '$app/navigation';
  import type { BeforeNavigate } from '@sveltejs/kit';
  import { base, resolve } from '$app/paths';
  import { ConfirmDialog } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let {
    enabled,
    hasChanges
  }: {
    /** False permits navigation after permission loss and hides any pending dialog. */
    enabled: boolean;
    /** Read current drafts at navigation time, including invalid input. */
    hasChanges: () => boolean | undefined;
  } = $props();

  let pendingNavigation = $state<BeforeNavigate | null>(null);
  // Approval applies only to the resumed destination, not to later navigation attempts.
  let approvedDestination: string | null = null;
  let approvedUnload = false;

  beforeNavigate((navigation) => {
    const { cancel, type, to } = navigation;
    if (type === 'leave' && approvedUnload) {
      approvedUnload = false;
      return;
    }
    if (to && approvedDestination === to.url.href) {
      approvedDestination = null;
      return;
    }
    // Permission loss must still be able to remove this editor and its private drafts.
    if (!enabled) {
      pendingNavigation = null;
      return;
    }
    if (!hasChanges()) return;
    cancel();
    // Closing or reloading a tab must use the browser's native unload warning.
    if (type !== 'leave' && to) pendingNavigation ??= navigation;
  });

  /** Resumes the selected destination without prompting twice or adding a back-history entry. */
  async function discardAndLeave(): Promise<void> {
    const navigation = pendingNavigation;
    pendingNavigation = null;
    if (!navigation?.to) return;
    if (navigation.type === 'popstate' && navigation.delta) {
      approvedDestination = navigation.to.url.href;
      window.history.go(navigation.delta);
    } else if (navigation.willUnload) {
      approvedUnload = true;
      window.location.assign(navigation.to.url.href);
    } else {
      approvedDestination = navigation.to.url.href;
      try {
        const { pathname, search, hash } = navigation.to.url;
        // Resolve the app-relative path; the captured URL already includes the deployment base.
        const path = pathname.slice(base.length) || '/';
        await goto(resolve(`${path}${search}${hash}` as '/'));
      } catch {
        // Another navigation guard can still cancel. Keep this page's drafts intact.
      } finally {
        approvedDestination = null;
      }
    }
  }
</script>

{#if enabled && pendingNavigation}
  <ConfirmDialog
    title={m('admin.rooms_admin.settings.discard_changes')}
    tone="warning"
    actionLabel={m('admin.rooms_admin.settings.discard_changes')}
    onconfirm={discardAndLeave}
    onclose={() => (pendingNavigation = null)}
  >
    {m('admin.rooms_admin.settings.unsaved_changes')}
  </ConfirmDialog>
{/if}
