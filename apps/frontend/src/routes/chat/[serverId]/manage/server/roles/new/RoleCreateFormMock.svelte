<script lang="ts">
  let {
    name = $bindable(),
    displayName = $bindable(),
    description = $bindable(),
    pingable = $bindable(),
    onSubmit
  }: {
    name: string;
    displayName: string;
    description: string;
    pingable: boolean;
    onSubmit: () => void;
  } = $props();

  function submit() {
    name = 'moderator';
    displayName = 'Moderator';
    description = 'Helps moderate the server';
    pingable = true;
    onSubmit();
  }
</script>

<!-- The data attributes read the bound values. Without a read, ESLint's
     no-useless-assignment rule reports the write-only $bindable props
     (sveltejs/eslint-plugin-svelte#1478). -->
<button
  data-testid="create-role"
  data-name={name}
  data-display-name={displayName}
  data-description={description}
  data-pingable={pingable}
  onclick={submit}>Create role</button
>
