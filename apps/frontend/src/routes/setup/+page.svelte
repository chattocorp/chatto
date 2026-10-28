<script lang="ts">
  import { passwordValidationError } from '$lib/passwordValidation';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { completeServerSetup } from '$lib/api/setup';
  import { getPublicServerInfo } from '@chatto/client/api/server';
  import { m } from '$lib/i18n/messages';
  import chattoIcon from '$lib/assets/chatto-icon.png';
  import { PageTitle, Hint, PaneHeader, CenteredPage, Panel, FormSection } from '$lib/ui';
  import { TextInput, TextArea, Button, Form } from '$lib/ui/form';

  let { data } = $props();
  let serverName = $state('');
  let description = $state('');
  let login = $state('');
  let displayName = $state('');
  let password = $state('');
  let passwordConfirmation = $state('');
  let fieldErrors = $state<Record<string, string>>({});
  let busy = $state(false);
  let error = $state('');
  let closed = $state(false);
  const confirmationError = $derived(
    passwordConfirmation && password !== passwordConfirmation
      ? m('common.validation.passwords_match')
      : undefined
  );

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;

    error = '';
    fieldErrors = {};
    const passwordError = passwordValidationError(password);
    if (passwordError) {
      fieldErrors.password = passwordError;
      return;
    }
    if (password !== passwordConfirmation) {
      return;
    }
    busy = true;
    try {
      await completeServerSetup(window.location.origin, {
        serverName,
        description,
        login,
        displayName,
        password
      });
      password = '';
      passwordConfirmation = '';
      // Use normal sign-in; setup never mints a second kind of session.
      await goto(resolve('/login'), { invalidateAll: true });
    } catch (cause) {
      // A failed response can follow a committed batch. Re-read discovery
      // before offering a retry, and retain ordinary sign-in as recovery.
      try {
        closed = !(await getPublicServerInfo(window.location.origin)).setupRequired;
      } catch {
        /* Keep the draft for a retry. */
      }
      if (closed) {
        password = '';
        passwordConfirmation = '';
      }
      const field = cause instanceof ConnectError ? cause.metadata.get('Chatto-Error-Field') : null;
      if (
        !closed &&
        cause instanceof ConnectError &&
        field &&
        ['login', 'display_name', 'password'].includes(field)
      ) {
        fieldErrors[field] = cause.rawMessage;
      } else {
        error =
          cause instanceof ConnectError && cause.code === Code.InvalidArgument
            ? cause.rawMessage
            : m('auth.setup.failed');
      }
    } finally {
      busy = false;
    }
  }
</script>

<!-- @component Welcomes the installer before origin-server setup in the shared client shell. -->
<PageTitle title={m('auth.setup.title')} />
<CenteredPage>
  {#snippet header()}
    {#if data.step !== 'welcome' || !data.setupServer.directLoginEnabled}
      <PaneHeader title={m('auth.setup.title')} backHref={resolve('/setup')} />
    {/if}
  {/snippet}
  {#if data.step === 'welcome' && data.setupServer.directLoginEnabled}
    <div class="mx-auto flex w-full max-w-3xl flex-col items-center text-center">
      <img
        src={chattoIcon}
        alt=""
        width="112"
        height="112"
        class="mb-8 size-24 outline-none sm:mb-10 sm:size-28"
      />
      <h1
        class="text-4xl leading-tight font-semibold tracking-tight text-balance text-text-top sm:text-5xl lg:text-6xl"
      >
        {m('add_server.directory.welcome_title')}
      </h1>
      <p
        class="mt-6 max-w-2xl text-2xl leading-relaxed font-semibold text-pretty whitespace-pre-line text-muted sm:text-3xl"
      >
        {m('auth.setup.intro')}
      </p>
      <div class="mt-10">
        <Button size="lg" href={`${resolve('/setup')}?step=details`}>
          {m('auth.setup.title')}
        </Button>
      </div>
    </div>
  {:else}
    <Panel title={m('auth.setup.title')}>
      {#if closed}
        <div class="flex flex-col items-start gap-4">
          <Hint>{m('auth.setup.closed')}</Hint>
          <Button href={resolve('/login')}>{m('auth.login.title')}</Button>
        </div>
      {:else if !data.setupServer.directLoginEnabled}
        <Hint tone="warning">{m('auth.setup.login_disabled')}</Hint>
      {:else}
        <Form onsubmit={submit} {error}>
          <div class="grid gap-8 md:grid-cols-2">
            <FormSection title={m('auth.setup.server_details')}>
              <div class="flex flex-col gap-4">
                <TextInput
                  id="setup-server-name"
                  label={m('auth.setup.server_name')}
                  description={m('auth.setup.server_name_help')}
                  bind:value={serverName}
                  required
                  maxlength={80}
                  disabled={busy}
                />
                <TextArea
                  id="setup-description"
                  label={m('auth.setup.description')}
                  description={m('auth.setup.description_help')}
                  bind:value={description}
                  maxlength={500}
                  rows={4}
                  disabled={busy}
                />
                <p class="text-sm text-muted">{m('auth.setup.change_later')}</p>
              </div>
            </FormSection>
            <FormSection title={m('auth.setup.owner')}>
              <div class="flex flex-col gap-4">
                <TextInput
                  id="setup-login"
                  error={fieldErrors.login}
                  oninput={() => {
                    delete fieldErrors.login;
                  }}
                  label={m('common.username')}
                  bind:value={login}
                  autocomplete="username"
                  maxlength={32}
                  required
                  disabled={busy}
                />
                <TextInput
                  id="setup-display-name"
                  error={fieldErrors.display_name}
                  oninput={() => {
                    delete fieldErrors.display_name;
                  }}
                  label={m('auth.setup.display_name')}
                  bind:value={displayName}
                  maxlength={32}
                  required
                  disabled={busy}
                />
                <TextInput
                  id="setup-password"
                  error={fieldErrors.password}
                  oninput={() => {
                    delete fieldErrors.password;
                  }}
                  label={m('common.password')}
                  type="password"
                  bind:value={password}
                  autocomplete="new-password"
                  placeholder={m('common.password_bytes_placeholder')}
                  required
                  disabled={busy}
                />
                <TextInput
                  id="setup-password-confirmation"
                  label={m('common.confirm_password')}
                  type="password"
                  bind:value={passwordConfirmation}
                  autocomplete="new-password"
                  error={confirmationError}
                  required
                  disabled={busy}
                />
              </div>
            </FormSection>
          </div>
          {#snippet footer()}
            <div class="flex w-full justify-end">
              <Button type="submit" loading={busy} disabled={busy}>
                <span class="iconify icon-[uil--rocket]" aria-hidden="true"></span>
                {m('auth.setup.finish')}
              </Button>
            </div>
          {/snippet}
        </Form>
      {/if}
    </Panel>
  {/if}
</CenteredPage>
