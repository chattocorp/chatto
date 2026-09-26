<!--
@component

A compact search field for chat surfaces. It shares the message composer's
height, radius, background, and spacing while keeping native form behaviour.
Use the bordered appearance for page searches, matching standard form inputs.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  const generatedId = $props.id();
  let {
    id = generatedId,
    testid,
    label,
    placeholder,
    value = $bindable(''),
    disabled = false,
    focusOnMount = false,
    onMountFocus,
    appearance = 'chat',
    clearLabel,
    oninput,
    onsubmit,
    onclear
  }: {
    id?: string;
    testid?: string;
    label: string;
    placeholder?: string;
    value?: string;
    disabled?: boolean;
    focusOnMount?: boolean;
    /** Called after a requested mount focus reaches the input. */
    onMountFocus?: () => void;
    /** Page searches use the standard bordered input; chat rails use the shell surface. */
    appearance?: 'chat' | 'bordered';
    clearLabel?: string;
    oninput?: (event: Event) => void;
    onsubmit?: () => void;
    onclear?: () => void;
  } = $props();

  let inputElement: HTMLInputElement | null = null;

  onMount(() => {
    const input = inputElement;
    if (!focusOnMount || !input || input.disabled) return;
    input.focus();
    if (document.activeElement === input) onMountFocus?.();
  });

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    onsubmit?.();
  }

  function clear(): void {
    value = '';
    onclear?.();
    inputElement?.focus();
  }
</script>

<form
  class={[
    'relative flex min-w-0 items-center gap-1 px-2.5 transition-opacity duration-100',
    appearance === 'bordered'
      ? 'h-10 control-frame bg-input focus-within:border-action'
      : 'h-12 chat-input-surface py-1.5',
    disabled && 'opacity-50'
  ]}
  onsubmit={submit}
>
  <label class="sr-only" for={id}>{label}</label>
  <span class="iconify icon-[uil--search] h-5 w-5 shrink-0 text-muted" aria-hidden="true"></span>
  <input
    bind:this={inputElement}
    {id}
    data-testid={testid}
    type="search"
    bind:value
    {placeholder}
    {disabled}
    autocomplete="off"
    class="search-cancel-hidden min-w-0 flex-1 bg-transparent px-1 py-1.5 text-text outline-none placeholder:text-muted/70"
    {oninput}
  />
  {#if clearLabel && value}
    <button
      type="button"
      class="inline-flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color] feedback-quick hover:bg-surface-emphasized hover:text-text focus-visible:bg-surface-emphasized focus-visible:outline-2 focus-visible:outline-action active:bg-surface-selected disabled:cursor-not-allowed disabled:opacity-50"
      aria-label={clearLabel}
      title={clearLabel}
      {disabled}
      onclick={clear}
    >
      <span class="iconify icon-[uil--times] text-base" aria-hidden="true"></span>
    </button>
  {/if}
</form>
