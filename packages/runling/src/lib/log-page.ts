/** Bounded output pages. Cursors count output entries across the complete run journal,
 * including earlier execution attempts. Filtering never changes an entry's cursor. */
export interface OutputEntry {
  cursor: number;
  timestamp: number;
  text: string;
  /** The next entry continues this text without a newline. */
  more?: boolean;
  activityId?: string;
}

export interface OutputPage {
  entries: OutputEntry[];
  /** Resume after this cursor, including records outside the selected activity. */
  next: number;
  hasEarlier: boolean;
  hasLater: boolean;
}

export const OUTPUT_PAGE_SIZE = 200;
export const OUTPUT_PAGE_BYTES = 256 * 1024;
