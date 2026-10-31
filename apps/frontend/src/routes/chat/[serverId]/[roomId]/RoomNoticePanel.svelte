<!-- @component Current notice and an explicit correction/republish editor. Stale drafts stay visible on conflict. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { pushState } from '$app/navigation';
  import { page } from '$app/state';
  import type { RoomNoticeStore } from '@chatto/client/room/notice';
  import { MAX_ROOM_NOTICE_LENGTH } from '@chatto/client/api/roomNotices';
  import { Code, ConnectError } from '@connectrpc/connect';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import { Button } from '$lib/ui/form';
  import MarkdownEditorField from '$lib/components/composer/MarkdownEditorField.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { ConfirmDialog, EmptyState, FormDialog, LoadingFog, LoadRetry } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { errorMessage } from '$lib/utils/errorMessage';
  import type { MarkdownMentionOptions } from '$lib/components/composer/mentionOptions';

  let {
    store,
    serverId,
    canManage = false,
    trigger,
    mentions
  }: {
    store: RoomNoticeStore;
    serverId: string;
    canManage?: boolean;
    /** Stable custom view in place of the sidebar preview. Open is absent without editing access. */
    trigger?: Snippet<[open: (() => void) | undefined]>;
    mentions?: MarkdownMentionOptions;
  } = $props();
  const editorId = $props.id();
  let draft = $state<{ roomId: string; epoch: number; body: string; version: string } | null>(null);
  let saving = $state(false);
  let confirmingClear = $state(false);
  let error = $state<unknown>(null);
  const notice = $derived(store.notice);
  const modal = $derived(page.state.modal);
  const ownsModal = $derived(
    modal?.type === 'roomNoticeEditor' &&
      modal.editorId === editorId &&
      modal.serverId === serverId &&
      modal.roomId === store.roomId
  );
  const editing = $derived(
    ownsModal && canManage && draft?.roomId === store.roomId && draft.epoch === store.accessEpoch
  );
  const valid = $derived(
    draft && draft.body.trim() && [...draft.body].length <= MAX_ROOM_NOTICE_LENGTH
  );
  const conflict = $derived(error instanceof ConnectError && error.code === Code.Aborted);

  function edit() {
    if (!notice) return;
    confirmingClear = false;
    draft = {
      roomId: store.roomId,
      epoch: store.accessEpoch,
      body: notice.body,
      version: notice.version
    };
    error = null;
    if (!ownsModal)
      pushState('', {
        modal: { type: 'roomNoticeEditor', serverId, roomId: store.roomId, editorId }
      });
  }
  function close() {
    confirmingClear = false;
    draft = null;
    error = null;
    if (ownsModal) history.back();
  }
  async function save(republish: boolean, clear = false) {
    if (!draft || saving) return;
    const submitted = draft;
    const target = store;
    saving = true;
    error = null;
    try {
      if (clear) await target.clear(submitted.version);
      else await target.publish(submitted.body, submitted.version, republish);
      if (draft === submitted) close();
    } catch (cause) {
      if (draft === submitted && target.accessEpoch === submitted.epoch) error = cause;
    } finally {
      saving = false;
    }
  }
  async function reload() {
    const target = store;
    await target.refresh();
    if (target === store && !target.error) edit();
  }
</script>

{#if store.supported}
  {#if trigger}
    {@render trigger(canManage && notice ? edit : undefined)}
  {:else}
    <div class="flex flex-col gap-3 px-2 py-3">
      {#if store.error}
        <LoadRetry message={errorMessage(store.error)} onretry={() => store.refresh()} />
      {:else if !notice}
        <LoadingFog class="min-h-24" label={m('common.loading')} />
      {:else if notice.body}
        <div class="min-w-0" dir="auto">
          <MessageContent
            mentionStore={mentions?.memberStore}
            body={notice.body}
            members={mentions?.members}
            roleHandles={mentions?.roles?.map((role) => role.name)}
            viewerLogin={mentions?.viewerLogin}
            onMentionClick={mentions?.onMentionClick}
          />
        </div>
      {:else}
        <EmptyState icon="icon-[uil--megaphone]" title={m('room.notice.empty')} />
      {/if}
      {#if canManage && notice}
        <div>
          <Button variant="secondary" fullWidth onclick={edit}>
            <span aria-hidden="true" class="iconify icon-[uil--edit]"></span>
            {m('room.notice.edit')}
          </Button>
        </div>
      {/if}
    </div>
  {/if}
  {#if editing && draft}
    <FormDialog
      visible
      title={m('room.notice.edit')}
      submitLabel={m('room.notice.republish')}
      submitIcon="iconify icon-[uil--megaphone]"
      loading={saving}
      disabled={!valid || conflict || draft.body === notice?.body}
      error={error ? (conflict ? m('room.notice.conflict') : errorMessage(error)) : null}
      onsubmit={() => save(true)}
      onclose={close}
    >
      <MarkdownEditorField
        {mentions}
        editorKind={userPreferences.composerEditor}
        label={m('room.notice.body')}
        placeholder={m('room.notice.body')}
        testid="room-notice-editor"
        bind:value={draft.body}
        maxlength={MAX_ROOM_NOTICE_LENGTH}
        tooLongMessage={m('room.notice.too_long', { max: MAX_ROOM_NOTICE_LENGTH })}
        disabled={saving}
        description={m('room.notice.help')}
      />
      {#if conflict}<Button variant="secondary" onclick={reload} disabled={saving}
          >{m('room.notice.reload')}</Button
        >{/if}
      {#snippet secondaryActions()}
        {#if notice?.body}
          <Button
            variant="danger-secondary"
            disabled={saving || conflict}
            onclick={() => (confirmingClear = true)}
            ><span aria-hidden="true" class="iconify icon-[uil--trash-alt]"></span>{m(
              'room.notice.clear'
            )}</Button
          >
          <Button
            variant="secondary"
            disabled={saving || !valid || conflict || draft?.body === notice?.body}
            onclick={() => save(false)}
            ><span aria-hidden="true" class="iconify icon-[uil--check]"></span>{m(
              'room.notice.save'
            )}</Button
          >
        {/if}
      {/snippet}
    </FormDialog>
    {#if confirmingClear}
      <ConfirmDialog
        title={m('room.notice.clear_confirm_title')}
        actionLabel={m('room.notice.clear')}
        actionIcon="iconify icon-[uil--trash-alt]"
        onconfirm={() => {
          confirmingClear = false;
          void save(false, true);
        }}
        onclose={() => (confirmingClear = false)}
      >
        {m('room.notice.clear_confirm_description')}
      </ConfirmDialog>
    {/if}
  {/if}
{/if}
