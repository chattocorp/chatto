package inspection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLivePayloadsAreNotSerializedAndDoNotEraseEvents(t *testing.T) {
	old := Turn{Number: 1, Steps: []Step{{Kind: "model", Detail: "completed", Request: strings.Repeat("a", 3<<20)}}}
	latest := Turn{Number: 2, Steps: []Step{{Kind: "model", Request: strings.Repeat("b", 2<<20)}}}
	retained := Retain([]Turn{old, latest})
	if len(retained) != 2 || retained[0].Steps[0].Request != "" || retained[0].Steps[0].Detail != "completed" || retained[1].Steps[0].Request == "" {
		t.Fatal("retention lost events or latest capture")
	}
	if old.Steps[0].Request == "" {
		t.Fatal("retention mutated another snapshot")
	}
	b, err := json.Marshal(latest)
	if err != nil || strings.Contains(string(b), `"Request":`) || len(b) > 1000 {
		t.Fatal("live payload serialized", err)
	}
}
