/** Reject missing directories before a tool can use the process directory. */
export function requireDirectory(directory: string): string {
  if (typeof directory !== "string" || directory.trim() === "") {
    throw new TypeError("An explicit directory is required");
  }
  return directory;
}
