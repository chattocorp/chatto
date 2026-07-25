export type ReactionSummaryView = {
  emoji: string;
  count: number;
  hasReacted: boolean;
  users: Array<{ id: string; displayName: string }>;
};
