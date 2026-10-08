package mismatch

import "hmans.de/chatto/pkg/events/intern"

type userKind struct{}
type roomKind struct{}

var users intern.Table[userKind]
var rooms intern.Table[roomKind]

// Both an assignment and resolution through the wrong kind must fail.
var wrongAssignment intern.ID[roomKind] = users.Intern("account-1")
var wrongResolution = rooms.Resolve(users.Intern("account-1"))
