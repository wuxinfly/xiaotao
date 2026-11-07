package agent

import (
	"context"
	"testing"
)

func TestFinalAnswerDoesNotRegenerate(t *testing.T) {
	svc := NewService(t.TempDir(), "unused", nil, nil, nil)
	history, _ := svc.Create(Options{Model: "test"})
	model := &fakeModel{replies: []*completion{{Content: "已有最终回答"}}}
	ends := 0
	err := svc.loop(context.Background(), nil, model, history, nil, func(e Event) {
		if e.Type == "answer_end" {
			ends++
		}
	})
	if err != nil || len(model.messages) != 1 || ends != 1 {
		t.Fatalf("compatibility path: %v %+v", err, model.messages)
	}
}
