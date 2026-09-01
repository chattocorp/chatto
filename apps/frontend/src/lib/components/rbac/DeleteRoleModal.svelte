<script lang="ts">
  import { ConfirmDialog } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let {
    roleDisplayName,
    deleting = false,
    onConfirm,
    onCancel
  }: {
    roleDisplayName: string;
    deleting?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();

  let visible = $state(true);

  function handleClose() {
    visible = false;
    onCancel();
  }
</script>

<ConfirmDialog
  {visible}
  title={m('rbac.delete_role.title')}
  actionLabel={m('rbac.delete_role.action')}
  actionLoadingLabel={m('rbac.delete_role.deleting')}
  actionIcon="iconify icon-[uil--trash-alt]"
  loading={deleting}
  onconfirm={onConfirm}
  onclose={handleClose}
>
  <p class="mb-4 text-muted">
    {m('rbac.delete_role.prompt', { role: roleDisplayName })}
  </p>
  <ul class="mb-4 list-inside list-disc text-sm text-muted">
    <li>{m('rbac.delete_role.remove_from_users')}</li>
    <li>{m('rbac.delete_role.delete_grants')}</li>
  </ul>
  <p class="text-sm font-medium text-error">{m('rbac.delete_role.irreversible')}</p>
</ConfirmDialog>
