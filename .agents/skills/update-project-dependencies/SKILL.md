---
name: "update-project-dependencies"
description: "Use this if you want to update all project dependencies while respecting semver compatibility."
---

# Skill: Update Project Dependencies

This skill updates all project dependencies while respecting semver compatibility, then runs tests to verify everything still works.

## Instructions

Follow these steps in order:

### 1. Capture Current State

Before making any changes, read and save the current dependency versions:

- Read `cli/go.mod` - note the versions of direct dependencies (the `require` blocks without `// indirect`)
- Read `frontend/package.json` - note the versions in `devDependencies`

### 2. Update Go Dependencies

Run these commands in the `cli/` directory:

```bash
cd cli && go get -u ./...
cd cli && go mod tidy
```

Note: `go get -u` updates to the latest minor/patch versions respecting the module's compatibility guarantees.

### 3. Update npm Dependencies

Run this command in the `frontend/` directory:

```bash
cd frontend && pnpm update
```

This updates packages within their semver ranges defined in package.json.

### 4. Run Tests

Execute the full test suite:

```bash
mise run test
```

**If tests fail:** Stop here and report the failures. Do NOT attempt to rollback the changes - leave them in place so the user can review and fix manually.

### 5. Generate Report

After completing the updates, provide a summary report with this structure:

## Dependency Update Report

### Go Dependencies (cli/)

| Package      | Previous | Updated |
| ------------ | -------- | ------- |
| package-name | v1.2.3   | v1.2.5  |

### npm Dependencies (frontend/)

| Package      | Previous | Updated |
| ------------ | -------- | ------- |
| package-name | ^1.2.3   | ^1.2.5  |

### Test Results

- Status: PASSED/FAILED
- If failed: List the failing tests and error messages

### Notable Updates

Highlight any significant updates, especially:

- Security-related packages
- Major framework updates (gin, svelte, sveltekit, etc.)
- Build tooling (vite, typescript, etc.)
