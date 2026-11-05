<script lang="ts">
  let { roleName }: { roleName: string } = $props();
</script>

<div data-testid="role-permissions" data-role-name={roleName}></div>
