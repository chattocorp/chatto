<!--
@component

A compact one-of-many mode switch. Use this for alternate views, filters, and
sort modes that belong together, such as “All / Unread” or
“Most relevant / Newest”. Native radio inputs provide keyboard navigation and
selection semantics; the surrounding pill presents the options as one control.

Use `ToggleChip` instead when choices can be toggled independently.
-->
<script lang="ts" generics="T extends string | number">
  import type { Attachment } from 'svelte/attachments';
  import type { ClassValue } from 'svelte/elements';

  let {
    label,
    options,
    value,
    onchange,
    disabled = false,
    class: className
  }: {
    /** Accessible name for the group. */
    label: string;
    options: ReadonlyArray<{ value: T; label: string; disabled?: boolean }>;
    value: T;
    onchange: (value: T) => void;
    disabled?: boolean;
    /** Layout-only classes such as responsive visibility or width. */
    class?: ClassValue;
  } = $props();

  const controlId = $props.id();
  const groupName = `segmented-control-${controlId}`;

  function selectedIndicator(_selectedValue: T): Attachment<HTMLFieldSetElement> {
    return (node) => {
      let frame = 0;

      function update() {
        cancelAnimationFrame(frame);
        frame = requestAnimationFrame(() => {
          const selectedLabel = node
            .querySelector<HTMLInputElement>('input:checked')
            ?.closest<HTMLElement>('label');
          if (!selectedLabel) return;

          const controlBounds = node.getBoundingClientRect();
          const labelBounds = selectedLabel.getBoundingClientRect();
          node.style.setProperty(
            '--segmented-indicator-x',
            `${labelBounds.left - controlBounds.left}px`
          );
          node.style.setProperty('--segmented-indicator-width', `${labelBounds.width}px`);
          node.dataset.indicatorReady = 'true';
        });
      }

      const resizeObserver = new ResizeObserver(update);
      resizeObserver.observe(node);
      node.querySelectorAll('label').forEach((label) => resizeObserver.observe(label));
      update();

      return () => {
        cancelAnimationFrame(frame);
        resizeObserver.disconnect();
      };
    };
  }
</script>

<fieldset
  class={[
    'group/segmented relative inline-flex h-10 w-fit min-w-0 items-center gap-px control-frame bg-input p-px',
    className
  ]}
  {disabled}
  {@attach selectedIndicator(value)}
>
  <legend class="sr-only">{label}</legend>
  <span
    aria-hidden="true"
    class="pointer-events-none absolute inset-y-px left-0 rounded bg-surface-selected opacity-0 transition-[width,transform,opacity] duration-[var(--motion-duration-pane)] ease-[var(--ease-out-expo)] [transform:translateX(var(--segmented-indicator-x,0px))] [width:var(--segmented-indicator-width,0px)] group-data-[indicator-ready=true]/segmented:opacity-100 motion-reduce:transition-none"
  ></span>

  {#each options as option, index (option.value)}
    <label class="relative flex min-w-0 cursor-pointer">
      <input
        class="peer absolute inset-0 z-20 m-0 h-full w-full cursor-pointer appearance-none rounded-full opacity-0 disabled:cursor-not-allowed"
        type="radio"
        name={groupName}
        value={String(option.value)}
        checked={value === option.value}
        disabled={disabled || option.disabled}
        onchange={() => onchange(option.value)}
      />
      <span
        class={[
          'relative z-10 inline-flex min-h-9 min-w-10 items-center justify-center px-3 text-sm font-medium text-muted transition-[background-color,color] duration-150 peer-checked:bg-surface-selected peer-checked:text-text-top peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-action peer-disabled:cursor-not-allowed peer-disabled:opacity-60 peer-[:not(:checked):hover]:bg-surface-emphasized/50 peer-[:not(:checked):hover]:text-text group-data-[indicator-ready=true]/segmented:peer-checked:bg-transparent',
          index === 0 ? 'rounded-s' : '',
          index === options.length - 1 ? 'rounded-e' : ''
        ]}
      >
        {option.label}
      </span>
    </label>
  {/each}
</fieldset>
