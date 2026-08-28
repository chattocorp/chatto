<script lang="ts">
  import type { AdminMember } from '$lib/api-client/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { Panel } from '$lib/ui';
  import { Hint } from '$lib/ui';
  import { Button, FormError, TextInput } from '$lib/ui/form';

  let {
    member,
    cancelHref,
    deleteMember
  }: {
    member: AdminMember;
    cancelHref: string;
    deleteMember: () => Promise<void>;
  } = $props();

  /** Wraps an interpolated user value in Unicode isolates so names and logins
   * cannot scramble surrounding RTL sentences. Built from char codes to keep
   * invisible bidirectional control characters out of this source file. */
  function isolate(value: string): string {
    return `${String.fromCharCode(0x2068)}${value}${String.fromCharCode(0x2069)}`;
  }

  let confirmText = $state('');
  let error = $state('');
  let deleting = $state(false);

  const canConfirm = $derived(!deleting && confirmText.length > 0 && confirmText === member.login);

  async function handleSubmit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canConfirm) return;
    deleting = true;
    error = '';

    try {
      await deleteMember();
      // On success the parent navigates away; keep the busy state while it does.
    } catch (err) {
      error = err instanceof Error ? err.message : m('admin.member_delete.failed');
      // Keep the typed confirmation so a retry needs no retyping.
      deleting = false;
    }
  }
</script>

<!-- @component Confirmation form for permanently deleting a member's account. Owns only the input state; the parent route owns authorization, the API call, and post-deletion navigation. -->
<Panel title={m('admin.members.danger_zone')} icon="iconify icon-[uil--exclamation-triangle]">
  <form class="flex max-w-md flex-col gap-4" onsubmit={handleSubmit}>
    <Hint tone="danger">
      <strong>{m('admin.member_delete.warning', { name: isolate(member.displayName) })}</strong>
    </Hint>

    <p class="text-sm text-muted">{m('admin.member_delete.consequences_intro')}</p>
    <ul class="list-inside list-disc text-sm text-muted">
      <li>{m('admin.member_delete.consequence_rooms')}</li>
      <li>{m('admin.member_delete.consequence_messages')}</li>
      <li>{m('admin.member_delete.consequence_profile')}</li>
      <li>{m('admin.member_delete.consequence_sessions')}</li>
    </ul>

    <TextInput
      id="member-delete-confirm"
      label={m('admin.member_delete.confirm_label', { login: isolate(member.login) })}
      bind:value={confirmText}
      placeholder={member.login}
      disabled={deleting}
      autocomplete="off"
    />

    {#if error}
      <FormError {error} />
    {/if}

    <div class="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" href={cancelHref} disabled={deleting}>
        {m('common.cancel')}
      </Button>
      <Button
        type="submit"
        variant="danger"
        defaultAction
        disabled={!canConfirm}
        loading={deleting}
        loadingText={m('admin.member_delete.deleting')}
      >
        {m('admin.member_delete.submit')}
      </Button>
    </div>
  </form>
</Panel>
