package sourceplugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDisplayIsOptionalAndPlain(t *testing.T) {
	var result Result
	if err := json.Unmarshal([]byte(`{"display":{"summary":"今日已签到","items":[{"label":"每日积分","value":"200","percent":50}]}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Display == nil || result.Display.Summary != "今日已签到" || len(result.Display.Items) != 1 || result.Display.Items[0].Percent == nil {
		t.Fatalf("display lost: %+v", result.Display)
	}
	var plain Result
	if err := json.Unmarshal([]byte(`{"value":{}}`), &plain); err != nil || plain.Display != nil {
		t.Fatal("display must stay optional")
	}
	// The host renders display strings as text; markup must not be interpreted.
	result.Display.Summary = "<b>bold</b>"
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `\u003cb\u003e`) {
		t.Fatalf("unexpected encoding: %s", raw)
	}
}
