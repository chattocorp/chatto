<script lang="ts">
  let { roleName, readOnly = false }: { roleName: string; readOnly?: boolean } = $props();
</script>

<div data-testid="role-permissions" data-role-name={roleName} data-read-only={readOnly}></div>
