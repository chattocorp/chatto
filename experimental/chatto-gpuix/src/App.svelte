<script lang="ts">
  import { onDestroy } from 'svelte';
  import Theme from './ui/Theme.svelte';
  import Gallery from './Gallery.svelte';
  import { inputTheme } from './ui/theme.ts';
  import ActionButton from './ActionButton.svelte';
  import Connected from './Connected.svelte';
  import Demo from './Demo.svelte';
  import { NativeSession } from './session.svelte.ts';

  let { session = new NativeSession() }: { session?: NativeSession } = $props();
  let demo = $state(false);
  let gallery = $state(false);
  onDestroy(() => session.close());
</script>

<Theme>
  {#if gallery}
    <Gallery onClose={() => (gallery = false)} />
  {:else if demo}
    <div class="demo-mode">
      <ActionButton label="Back to sign in" onclick={() => (demo = false)} />
      <div class="demo-body"><Demo /></div>
    </div>
  {:else if session.server}
    {#key session.draftGeneration}
      <Connected {session} server={session.server} />
    {/key}
  {:else}
    <div class="login-screen">
      <div class="form">
        <text class="title">Sign in to Chatto</text>
        <text class="subtitle">Enter your server and account.</text>
        {#if session.error}<text class="error">{session.error}</text>{/if}
        <text class="label">Server URL</text>
        <input
          value={session.origin}
          readonly={session.busy}
          theme={inputTheme}
          aria-label="Server URL"
          placeholder="https://chat.example.com"
          onchange={(event) => (session.origin = event.value)}
          testid="login-origin"
        />
        <text class="label">Username</text>
        <input
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
</Theme>

<style>
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
    width: 420px;
    gap: 14px;
    padding: 24px;
  }
  .title {
    font-size: 28px;
    font-weight: bold;
    color: var(--color-text-top);
  }
  .subtitle {
    font-size: 12px;
    color: var(--color-muted);
    white-space: normal;
  }
  .label {
    font-size: 13px;
    color: var(--color-text);
  }
  .error {
    font-size: 13px;
    color: var(--color-error);
    white-space: normal;
  }
</style>
