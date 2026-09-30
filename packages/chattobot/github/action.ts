/** A GitHub change as the host runs it and as the authorization classifier sees it. */
import { shellQuote } from './policy.ts';

/** A change to run. `args` excludes the program name. */
export interface GitHubAction {
  args: readonly string[];
  /** Body text, passed as `--body=`. */
  body?: string;
}

/** The complete argument list that runs, without the program name. */
export const actionArgs = (action: GitHubAction) =>
  // `--body=` keeps a body that starts with a dash from being read as a flag.
  action.body === undefined ? [...action.args] : [...action.args, `--body=${action.body}`];

/** The exact command line, quoted for a POSIX shell. */
export const renderCommand = (action: GitHubAction) =>
  ['gh', ...actionArgs(action)].map(shellQuote).join(' ');
