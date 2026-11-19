import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { StaleResponseError } from '@chatto/client/api/connect';
import { toast } from '$lib/ui/toast';
import { errorMessage, loadErrorMessage, toastError } from './errorMessage';

describe('errorMessage', () => {
  it.each([
    [Code.PermissionDenied, 'You do not have permission to do that.'],
    [Code.NotFound, 'The requested item could not be found.'],
    [Code.Unauthenticated, 'Your session has expired. Sign in again.'],
    [Code.Unavailable, 'Network error. Please try again.'],
    [Code.DeadlineExceeded, 'Network error. Please try again.'],
    [Code.Aborted, 'This changed while you were editing it. Reload the page, then try again.']
  ])('localizes code %s instead of showing server text', (code, expected) => {
    expect(errorMessage(new ConnectError('internal detail', code), 'Fallback')).toBe(expected);
  });

  it('localizes a fetch failure that Connect wraps as an unknown error', () => {
    const error = ConnectError.from(new TypeError('Failed to fetch'));
    expect(error.code).toBe(Code.Unknown);
    expect(errorMessage(error, 'Fallback')).toBe('Network error. Please try again.');
  });

  it.each([
    Code.InvalidArgument,
    Code.FailedPrecondition,
    Code.AlreadyExists,
    Code.OutOfRange,
    Code.ResourceExhausted
  ])('shows the server text without the code prefix for code %s', (code) => {
    expect(errorMessage(new ConnectError('name is too long', code), 'Fallback')).toBe(
      'name is too long'
    );
  });

  it('uses the fallback when a server-text code has no message', () => {
    expect(errorMessage(new ConnectError('', Code.InvalidArgument), 'Fallback')).toBe('Fallback');
  });

  it.each([Code.Internal, Code.Unknown, Code.Unimplemented, Code.Canceled])(
    'uses the fallback for code %s',
    (code) => {
      expect(errorMessage(new ConnectError('stack detail', code), 'Fallback')).toBe('Fallback');
    }
  );

  it('keeps the message of a plain error', () => {
    expect(errorMessage(new Error('Invalid login or password'), 'Fallback')).toBe(
      'Invalid login or password'
    );
  });

  it('uses the fallback for a non-error value or an empty message', () => {
    expect(errorMessage('boom', 'Fallback')).toBe('Fallback');
    expect(errorMessage(new Error(''), 'Fallback')).toBe('Fallback');
  });

  it('uses the generic message when no fallback is given', () => {
    expect(errorMessage(undefined)).toBe('Something went wrong');
  });
});

describe('loadErrorMessage', () => {
  it.each([Code.InvalidArgument, Code.FailedPrecondition, Code.ResourceExhausted])(
    'uses the fallback instead of server text for code %s',
    (code) => {
      expect(loadErrorMessage(new ConnectError('cursor detail', code), 'Fallback')).toBe(
        'Fallback'
      );
    }
  );

  it('localizes access and network codes like errorMessage', () => {
    expect(loadErrorMessage(new ConnectError('detail', Code.Unavailable), 'Fallback')).toBe(
      'Network error. Please try again.'
    );
  });
});

describe('toastError', () => {
  afterEach(() => vi.restoreAllMocks());

  it('shows the error message as an error toast', () => {
    const error = vi.spyOn(toast, 'error');
    toastError(new ConnectError('taken', Code.AlreadyExists));
    expect(error).toHaveBeenCalledWith('taken');
  });

  it('does not show a discarded stale response', () => {
    const error = vi.spyOn(toast, 'error');
    toastError(new StaleResponseError(true));
    expect(error).not.toHaveBeenCalled();
  });
});
