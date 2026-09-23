---
name: js-ts-authoring
description: Write or revise JavaScript and TypeScript source, including scripts, tests, and Svelte modules. Apply its documentation and Prettier practices.
---

# JavaScript and TypeScript authoring

Read the nearest `AGENTS.md` and follow the package's existing module, type, and test conventions. For Svelte components and modules, also use the repository's Svelte skills and checks.

## Write code that explains its behavior

- Keep modules focused. Give functions and values names that identify their role.
- Model input and output types at API boundaries. Narrow `unknown` before use. Avoid `any`, broad type assertions, and hidden mutable state when a precise type or explicit parameter is practical.
- Handle failure, cancellation, and cleanup at the owner of the operation. Make ordering and ownership clear when asynchronous work or subscriptions are involved.
- Give each nontrivial module a short comment that states its responsibility. Include lifetime or important constraints when they affect correct use.
- Document public functions, classes, types, and important fields with JSDoc or the package's established code documentation style. Explain the contract: what it does, significant inputs and outputs, errors, side effects, lifecycle, and invariants where relevant. Also document private helpers whose behavior is hard to infer.
- Add inline comments where a reader needs context to follow a branch, transformation, ordering requirement, protocol rule, or non-obvious calculation. State what the code does and why that behavior is required. Keep comments next to the code they describe and update them when behavior changes. Do not add a comment to every line or repeat a descriptive name.

## Format every source edit

Before finishing, run Prettier on every JavaScript or TypeScript source file you created or changed, including tests, scripts, and files produced by the agent. In the root pnpm workspace, pass the paths explicitly:

```sh
mise x -- pnpm format -- path/to/file.ts path/to/other-file.mjs
mise x -- pnpm format:check -- path/to/file.ts path/to/other-file.mjs
```

Format each changed Svelte file with the nearest package's Prettier configuration too. Do not hand-edit generated protobuf clients; change their source and regenerate them through the repository workflow. Run the relevant type, lint, and test checks after formatting. Report the checks that actually ran.
