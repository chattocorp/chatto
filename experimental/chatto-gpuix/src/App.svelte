<script lang="ts">
  import { on_window_key } from 'gpuix-svelte';
  import { appearance } from './ui/scale.svelte.ts';
  import { onDestroy } from 'svelte';
  import Theme from './ui/Theme.svelte';
  import Gallery from './Gallery.svelte';
  import { inputTheme } from './ui/theme.ts';
  import ActionButton from './ActionButton.svelte';
  import ServerGutter from './ServerGutter.svelte';
  import ManageServers from './ManageServers.svelte';
  import Connected from './Connected.svelte';
  import Demo from './Demo.svelte';
  import { NativeSession } from './session.svelte.ts';

  let { session = new NativeSession() }: { session?: NativeSession } = $props();
  let demo = $state(false);
  let gallery = $state(false);
  let passwordForm = $state(false);
  onDestroy(() => {
    void session.close();
  });
</script>

<Theme>
  <div
    class="shell"
    {@attach () =>
      on_window_key('keydown', (event) => {
        if (!(event.modifiers?.cmd || event.modifiers?.ctrl) || event.modifiers?.alt) return;
        const key = event.keyChar ?? event.key;
        if (key === '+' || key === '=') {
          event.preventDefault();
          session.setUiScale(appearance.scale + 0.1);
        } else if (key === '-') {
          event.preventDefault();
          session.setUiScale(appearance.scale - 0.1);
        } else if (key === '0') {
          event.preventDefault();
          session.setUiScale(1);
        }
      })}
  >
    <ServerGutter {session} />
    <div class="main">
      {#if session.booting}
        <div class="login-screen"><text>Opening saved servers…</text></div>
      {:else if gallery}
        <Gallery onClose={() => (gallery = false)} />
      {:else if demo}
        <div class="demo-mode">
          <ActionButton label="Back to sign in" onclick={() => (demo = false)} />
          <div class="demo-body"><Demo /></div>
        </div>
      {:else if session.managing}
        <ManageServers {session} />
      {:else if session.server}
        {#key `${session.activeServerId}:${session.draftGeneration}`}
          <Connected {session} server={session.server} />
        {/key}
      {:else}
        <div class="login-screen">
          <div class="form">
            <text class="title"
              >{session.activeRegistration && !session.adding
                ? `Sign in to ${session.activeRegistration.name}`
                : 'Add a Chatto server'}</text
            >
            <text class="subtitle"
              >Sign in through your server in the browser. Your other servers stay connected.</text
            >
            {#if session.error}<text class="error">{session.error}</text>{/if}
            {#if !session.storageReady}<ActionButton
                label="Retry opening saved sessions"
                onclick={() => session.retryOpening()}
              />{/if}
            <text class="label">Server URL</text>
            <input
              class="login-input"
              value={session.origin}
              readonly={session.busy}
              theme={inputTheme}
              aria-label="Server URL"
              placeholder="https://chat.example.com"
              onchange={(event) => (session.origin = event.value)}
              testid="login-origin"
            />
            <ActionButton
              label={session.busy ? 'Waiting for browser…' : 'Continue in browser'}
              primary
              disabled={session.busy || !session.storageReady || !session.origin.trim()}
              onclick={() => void session.signInWithBrowser()}
              testid="browser-sign-in"
            />
            {#if session.busy}<ActionButton
                label="Cancel sign-in"
                quiet
                onclick={() => session.cancelSignIn()}
              />{/if}
            <ActionButton
              label={passwordForm ? 'Hide password sign-in' : 'Use username and password'}
              quiet
              onclick={() => (passwordForm = !passwordForm)}
              testid="show-password"
            />
            {#if passwordForm}
              <text class="label">Username</text>
              <input
                class="login-input"
                value={session.login}
                readonly={session.busy}
                theme={inputTheme}
                aria-label="Username"
                placeholder="Username"
                onchange={(event) => (session.login = event.value)}
                onsubmit={() => void session.signIn()}
                testid="login-name"
              />
              <ActionButton
                label={session.busy ? 'Signing in…' : 'Sign in'}
                primary
                disabled={session.busy || !session.origin.trim() || !session.login.trim()}
                onclick={() => void session.signIn()}
                testid="sign-in"
              />
              <text class="subtitle">macOS will ask for your password in a secure dialog.</text>
            {/if}
            <ActionButton
              label="Try the local demo"
              disabled={session.busy}
              onclick={() => (demo = true)}
              testid="open-demo"
            />
            <ActionButton
              label="Component gallery"
              icon="gallery"
              quiet
              onclick={() => (gallery = true)}
              testid="open-gallery"
            />
          </div>
        </div>
      {/if}
    </div>
  </div>
</Theme>

<style>
  .login-input {
    font-family: var(--font-sans);
    font-size: var(--text-body);
  }
  .shell {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
  }
  .main {
    flex-grow: 1;
    min-width: 0;
    height: 100%;
  }
  .demo-mode {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    background-color: var(--color-background);
  }
  .demo-body {
    flex-grow: 1;
    min-height: 0;
  }

  .login-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    background-color: var(--color-background);
    color: var(--color-text);
  }
  .form {
    display: flex;
    flex-direction: column;
    width: var(--size-420);
    gap: var(--size-14);
    padding: var(--size-24);
  }
  .title {
    font-size: var(--text-display);
    font-weight: bold;
    color: var(--color-text-top);
  }
  .subtitle {
    font-size: var(--text-small);
    color: var(--color-muted);
    white-space: normal;
  }
  .label {
    font-size: var(--text-body);
    color: var(--color-text);
  }
  .error {
    font-size: var(--text-body);
    color: var(--color-error);
    white-space: normal;
  }
</style>
