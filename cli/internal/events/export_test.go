package events

import "time"

// This file exposes narrow test seams to the external-package contract suite.
// Production code cannot import these symbols.

func ExportAcquireProjectorApplyBarrier(projector *Projector) {
	projector.applyMu.Lock()
	projector.applyMu.Unlock()
}

func ExportSetSnapshotLoadTimeout(projector *Projector, timeout time.Duration) {
	projector.snapshotLoadTimeout = timeout
}

func ExportMaybeCompleteStartup(projector *Projector, now time.Time) {
	projector.maybeCompleteStartup(now)
}

func ExportSubjectMatchesFilter(filter, subject string) bool {
	return subjectMatchesFilter(filter, subject)
}

type ExportCompiledSubjectFilter struct {
	filter compiledSubjectFilter
}

func ExportCompileSubjectFilter(filter string) ExportCompiledSubjectFilter {
	return ExportCompiledSubjectFilter{filter: compileSubjectFilter(filter)}
}

func (f ExportCompiledSubjectFilter) Matches(subject string) bool {
	return f.filter.matches(subject)
}

func ExportStreamSequenceFromReply(reply string) (uint64, error) {
	return streamSequenceFromReply(reply)
}

func ExportProjectorConsumesSubject(projector *Projector, subject string) bool {
	return projector.consumesSubject(subject)
}
