package intern

// These estimates preserve the projection diagnostics used by the initial
// consumer. Go map and slice overhead can differ by runtime and allocation.
const (
	mapEntryOverhead        int64 = 64
	sliceEntryOverhead      int64 = 24
	compactMapEntryOverhead int64 = 16
)
