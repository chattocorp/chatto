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

<button data-testid="create-role" onclick={submit}>Create role</button>
