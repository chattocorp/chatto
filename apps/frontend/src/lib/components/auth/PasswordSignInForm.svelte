<!-- @component Shared password fields and submit feedback. The caller owns credentials and submission. -->
<script lang="ts">
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let {
    identifier = $bindable(''),
    password = $bindable(''),
    disabled = false,
    loading = false,
    error = '',
    compact = false,
    autofocus = false,
    onsubmit
  }: {
    identifier?: string;
    password?: string;
    disabled?: boolean;
    loading?: boolean;
    error?: string;
    compact?: boolean;
    autofocus?: boolean;
    onsubmit: (event: SubmitEvent) => void;
  } = $props();
  const id = $props.id();
</script>

<Form {onsubmit}>
  <TextInput
    id={`${id}-identifier`}
    label={m('auth.login.identifier_label')}
    bind:value={identifier}
    autocomplete="username"
    placeholder={m('common.email_placeholder')}
    required
    disabled={disabled || loading}
    {autofocus}
  />
  <TextInput
    id={`${id}-password`}
    label={m('common.password')}
    type="password"
    bind:value={password}
    autocomplete="current-password"
    placeholder={m('common.password_placeholder')}
    required
    disabled={disabled || loading}
  />
  {#if error}<Hint tone="danger">{error}</Hint>{/if}
  <Button
    type="submit"
    size={compact ? 'md' : 'lg'}
    fullWidth
    {loading}
    loadingText={m('auth.login.signing_in')}
    disabled={disabled || loading || !identifier.trim() || !password}
  >
    <span aria-hidden="true" class="iconify icon-[mdi--login]"></span>
    {m('common.sign_in')}
  </Button>
</Form>
