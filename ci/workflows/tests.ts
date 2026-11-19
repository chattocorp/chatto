/** Build the CI image once, then run Chatto's test tasks in isolated containers. */
import { fileURLToPath } from 'node:url';
import { task, Type, type Static } from 'runling';
import { docker } from 'runling/docker';
import { workingTreeManifest } from 'runling/git';
import { buildCIImage } from '../docker/image.ts';

const repository = fileURLToPath(new URL('../..', import.meta.url));
const Job = Type.Union([Type.Literal('backend'), Type.Literal('frontend')]);
const ImageInput = Type.Object({ image: Type.String() });

/** Both jobs get the same limits and a private writable copy of the mounted checkout. */
function container(image: string, files: string) {
  return {
    image,
    mounts: [
      { source: repository, target: '/repo', readOnly: true },
      { source: files, target: '/input/source-files', readOnly: true }
    ],
    cpus: 4,
    memory: '6g',
    timeoutMs: 30 * 60_000
  };
}

/** Run the repository's backend test task, including its build and smoke-test dependencies. */
export const testBackend = task(
  { name: 'Chatto Go tests', input: ImageInput, result: Type.Null() },
  async (ctx, { image }) => {
    await using source = await workingTreeManifest(ctx, repository);
    await docker.run(ctx, {
      ...container(image, source.path),
      command: ['mise', 'run', '--skip-tools', 'test-cli']
    });
    return null;
  }
);

/** Run the repository's frontend test task, including browser component tests. */
export const testFrontend = task(
  { name: 'Chatto frontend tests', input: ImageInput, result: Type.Null() },
  async (ctx, { image }) => {
    await using source = await workingTreeManifest(ctx, repository);
    await docker.run(ctx, {
      ...container(image, source.path),
      command: ['mise', 'run', '--skip-tools', 'test-frontend']
    });
    return null;
  }
);

export default task(
  {
    name: 'Chatto tests',
    input: Type.Object({
      jobs: Type.Optional(Type.Array(Job, { minItems: 1, uniqueItems: true }))
    }),
    result: Type.Object({ passed: Type.Array(Job) })
  },
  async (ctx, { jobs: selected }) => {
    const jobs: Static<typeof Job>[] = selected ?? ['backend', 'frontend'];
    const image = await buildCIImage(ctx, {});
    // Invocation handles cancel and await a pending sibling if either task fails.
    await using backend = jobs.includes('backend') ? ctx.start(testBackend, { image }) : undefined;
    await using frontend = jobs.includes('frontend')
      ? ctx.start(testFrontend, { image })
      : undefined;
    await Promise.all([backend?.result, frontend?.result]);
    return { passed: jobs };
  }
);
