/** A role that the composer can offer as a mention target. */
export type MentionRole = {
  name: string;
  /** Human-readable label from the public role catalogue. */
  displayName?: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
};
