package core

import (
	"strings"
	"testing"
)

func TestEvaluateESBootVerificationReportDetectsCountRegressions(t *testing.T) {
	report := &esBootVerificationReport{
		legacy: esLegacyCounts{
			rooms:               3,
			memberships:         7,
			roomGroups:          2,
			roomLayoutPresent:   true,
			serverConfigPresent: true,
			messages:            9,
			reactions:           4,
		},
		projected: esProjectedCounts{
			rooms:                  3,
			memberships:            6,
			roomGroups:             2,
			roomLayoutGroups:       0,
			serverConfigConfigured: false,
			messagePosts:           8,
			activeReactions:        4,
		},
		decodeErrors: 1,
	}

	var core ChattoCore
	core.evaluateESBootVerificationReport(report)

	assertProblemContains(t, report.problems, "memberships")
	assertProblemContains(t, report.problems, "messages")
	assertProblemContains(t, report.problems, "server config")
	assertProblemContains(t, report.problems, "room layout")
	assertProblemContains(t, report.problems, "decode errors")
}

func TestEvaluateESBootVerificationReportAllowsProjectedSuperset(t *testing.T) {
	report := &esBootVerificationReport{
		legacy: esLegacyCounts{
			rooms:       2,
			memberships: 3,
			roomGroups:  1,
			messages:    5,
			reactions:   1,
		},
		projected: esProjectedCounts{
			rooms:           3,
			memberships:     4,
			roomGroups:      2,
			messagePosts:    6,
			activeReactions: 1,
		},
	}

	var core ChattoCore
	core.evaluateESBootVerificationReport(report)

	if len(report.problems) != 0 {
		t.Fatalf("problems = %v, want none", report.problems)
	}
}

func assertProblemContains(t *testing.T, problems []string, needle string) {
	t.Helper()
	for _, problem := range problems {
		if strings.Contains(problem, needle) {
			return
		}
	}
	t.Fatalf("problems %v did not contain %q", problems, needle)
}
