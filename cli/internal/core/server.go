package core

// Server membership is implicit — every authenticated user counts as a
// member of the (single) server. There's nothing to write on signup,
// and there's no auto-join logic: room membership is strictly explicit
// (users join via `JoinRoom` or admin mass-invite). New users land in
// an empty sidebar and use the room directory to discover rooms.
