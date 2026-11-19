package core

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// sharedJobTypes lists every job type for the server and the bundled frontend
// (apps/frontend/src/routes/chat/[serverId]/manage/server/jobs/jobPresentation.test.ts),
// so that a new type cannot ship without a label on the Jobs page.
const sharedJobTypes = "../../../testdata/jobs/types.json"

func TestJobTypesMatchSharedFixture(t *testing.T) {
	data, err := os.ReadFile(sharedJobTypes)
	require.NoError(t, err)
	var fixture struct {
		Types []string `json:"types"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.ElementsMatch(t, fixture.Types, jobTypes(), "update testdata/jobs/types.json and the frontend job labels when a job type changes")
}
