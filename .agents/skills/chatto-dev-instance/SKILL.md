---
name: "chatto-dev-instance"
description: "Deploy a new version of Chatto to the dev instance Kubernetes server using Argo Rollouts."
---

# Deploy to Dev Server

Deploy a new version of Chatto to the dev Kubernetes cluster using Argo Rollouts with blue-green deployments.

## Arguments: $ARGUMENTS

Arguments can be:

- A version/tag (e.g., `v1.2.3`, `latest`, `sha-abc123`) - deploys that version
- `status` - shows current rollout status
- `promote` - promotes the preview to active
- `abort` - aborts the current rollout
- `undo` - rolls back to previous version

If no arguments are provided, prompt the user for what they want to do.

## Instructions

### Deploying a New Version

If a version tag is provided (anything that's not a command keyword):

1. Set the new image:

   ```bash
   kubectl argo rollouts set image chatto chatto=ghcr.io/hmans/chatto:<tag> -n chatto
   ```

2. Watch the rollout progress:

   ```bash
   kubectl argo rollouts get rollout chatto -n chatto
   ```

3. Inform the user that:
   - The preview service is now running the new version
   - They should verify the preview works correctly
   - Use `/chatto-dev-instance promote` to promote the new version to active
   - Use `/chatto-dev-instance abort` to cancel if something is wrong

### Checking Status

If the argument is `status`:

```bash
kubectl argo rollouts get rollout chatto -n chatto
```

Report the current state, including:

- Active revision and image
- Preview revision and image (if in progress)
- Rollout phase and status

### Promoting to Active

If the argument is `promote`:

1. Confirm with the user before promoting
2. Execute:
   ```bash
   kubectl argo rollouts promote chatto -n chatto
   ```
3. Show the updated status

### Aborting a Rollout

If the argument is `abort`:

1. Confirm with the user before aborting
2. Execute:
   ```bash
   kubectl argo rollouts abort chatto -n chatto
   ```
3. Show the updated status

### Rolling Back

If the argument is `undo`:

1. Confirm with the user before rolling back
2. Execute:
   ```bash
   kubectl argo rollouts undo chatto -n chatto
   ```
3. Show the updated status

## Common Workflow

A typical deployment workflow:

1. `/chatto-dev-instance v1.2.3` - Deploy version v1.2.3 to preview
2. Test the preview environment
3. `/chatto-dev-instance promote` - Promote to active if tests pass
4. Or `/chatto-dev-instance abort` - Cancel if something is wrong
