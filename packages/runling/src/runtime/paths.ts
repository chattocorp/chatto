/**
 * Format a path for display: strip the working directory prefix so paths
 * echoed in logs look like local (project-relative) paths.
 */
export function displayPath(path: string, cwd?: string): string {
  if (cwd === undefined) return path;
  if (path === cwd) return ".";
  if (path.startsWith(`${cwd}/`)) return path.slice(cwd.length + 1);
  return path;
}

/**
 * Best-effort removal of absolute working-directory prefixes from a free-form
 * string (e.g. an echoed shell command) so embedded paths read as relative.
 */
export function displayText(
  text: string,
  cwd?: string,
): string {
  if (cwd === undefined) return text;
  return text.replaceAll(`${cwd}/`, "");
}
