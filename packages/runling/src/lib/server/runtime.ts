// adapter-node bundles static self-imports. Resolve these at execution time so
// the console and application share runtime contexts and error constructors.
const runtimeModule = 'runling';
const webModule = 'runling/web';
export const runtime = (await import(/* @vite-ignore */ runtimeModule)) as typeof import('runling');
export const web = (await import(/* @vite-ignore */ webModule)) as typeof import('runling/web');
