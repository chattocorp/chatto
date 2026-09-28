/** A role that the composer can offer as a mention target. */
export type MentionRole = {
  name: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
};
