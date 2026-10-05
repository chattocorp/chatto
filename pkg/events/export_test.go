package events

import "time"

// SetStartupReconcileIntervalForTest shortens the startup reconciliation
// period so external tests do not wait for the production interval.
func (p *Projector) SetStartupReconcileIntervalForTest(interval time.Duration) {
	p.startupReconcileInterval = interval
}
