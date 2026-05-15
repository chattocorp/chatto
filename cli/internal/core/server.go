package core

// Server membership is implicit — every authenticated user counts as a
// member of the (single) server. There's nothing to write on signup.
//
// Previously this file held a `JoinServer` hook that auto-joined the
// new user to any channel room flagged `auto_join`. That model has been
// replaced by "global rooms" (Room.is_global), which grant implicit
// membership without any per-user KV record. See ADR-031 and the
// is_global rollout.
