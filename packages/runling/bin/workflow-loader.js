// Workflow/config TypeScript uses ESM imports even in a CommonJS project.
// Let tsx perform the transform; do not transform installed dependencies.
export function resolve(specifier, context, nextResolve) {
  const result = nextResolve(specifier, context);
  const url = new URL(result.url);
  if (url.protocol === "file:" && /\.(?:ts|tsx|mts)$/.test(url.pathname) && !url.pathname.includes("/node_modules/")) {
    return { ...result, format: "module" };
  }
  return result;
}
