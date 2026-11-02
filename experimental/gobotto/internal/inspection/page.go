package inspection

// Page is a detached window of the newest eligible turns in execution order. Before is
// the exclusive turn number for the next older page, or zero at the end.
type Page struct {
	Turns  []Turn
	Logs   []Log
	Before int
}
