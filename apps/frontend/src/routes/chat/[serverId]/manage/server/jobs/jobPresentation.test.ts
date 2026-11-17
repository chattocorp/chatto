/**
 * Checks the job labels against `testdata/jobs/types.json`. The server checks
 * the same list against its registered job types, so a new type cannot ship
 * without a label on the Jobs page.
 */
import { describe, expect, it } from 'vitest';
import sharedJobTypes from '../../../../../../../../../testdata/jobs/types.json';
import { knownJobTypeLabel } from './jobPresentation';

describe('knownJobTypeLabel', () => {
  it.each(sharedJobTypes.types)('labels the %s job type', (type) => {
    expect(knownJobTypeLabel(type)).not.toBeNull();
  });

  it('returns null for a future job type', () => {
    expect(knownJobTypeLabel('future_type')).toBeNull();
  });
});
