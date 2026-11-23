/** Submit a signed artifact once and retry temporary Apple ticket-service failures. */
import { execFileSync } from 'node:child_process';
import pRetry from 'p-retry';

/**
 * Submit a signed disk image and require Apple's explicit acceptance.
 *
 * Apple issues a ticket for the submitted file and for each nested code item,
 * so one disk image submission also notarises the app inside it.
 */
export function submitMacOSArtifact(file, credentials, run = execFileSync) {
  // notarytool can exit successfully for a rejected submission. Check its status.
  const submission = JSON.parse(
    run(
      'xcrun',
      [
        'notarytool',
        'submit',
        file,
        '--key',
        credentials.appleApiKey,
        '--key-id',
        credentials.appleApiKeyId,
        '--issuer',
        credentials.appleApiIssuer,
        '--wait',
        '--output-format',
        'json'
      ],
      { encoding: 'utf8' }
    )
  );
  console.log(`macOS notarisation ${submission.id}: ${submission.status}`);
  // Keep Apple's complete submission response in the CI artifact for diagnosis.
  console.log(JSON.stringify(submission, null, 2));
  if (submission.status !== 'Accepted') {
    throw new Error(`macOS notarisation was not accepted: ${submission.id}`);
  }
}

/** Staple and validate an accepted artifact without signing or submitting it again. */
export async function stapleMacOSArtifact(file, run = execFileSync) {
  for (const operation of ['staple', 'validate']) {
    await pRetry(() => run('xcrun', ['stapler', operation, file], { stdio: 'inherit' }), {
      retries: 4,
      minTimeout: 15_000,
      maxTimeout: 120_000,
      factor: 2,
      // Exit 68 is the CloudKit connection failure seen in release builds.
      // Missing tickets and signature failures must still stop publication.
      shouldRetry: ({ error }) => error.status === 68,
      onFailedAttempt: ({ error, attemptNumber, retriesLeft, retryDelay }) => {
        if (error.status === 68 && retriesLeft > 0) {
          console.warn(
            `stapler ${operation} attempt ${attemptNumber} failed with network error 68; retrying in ${retryDelay / 1000}s (${retriesLeft} retries left).`
          );
        }
      }
    });
  }
}
