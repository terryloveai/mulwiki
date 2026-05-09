package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tethy/mulwiki/server/internal/store"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

func TestTaskMessageAppendListAndDuplicateSeq(t *testing.T) {
	s, _, _ := newTestTaskService(t)
	task, err := s.CreateTaskForJob(context.Background(), "job-msg", "ws1", "a1", "rt1", "src1", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob: %v", err)
	}

	messages := []protocol.AgentTaskMessage{
		{
			Seq:       1,
			Type:      "text",
			Content:   "hello",
			SessionID: "sess-1",
		},
		{
			Seq:    2,
			Type:   "tool-use",
			Tool:   "exec_command",
			CallID: "call-1",
			Input:  json.RawMessage(`{"cmd":"git status"}`),
		},
	}
	if err := s.AppendMessages(context.Background(), "ws1", task.ID, messages); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}
	if err := s.AppendMessages(context.Background(), "ws1", task.ID, []protocol.AgentTaskMessage{messages[1]}); err != nil {
		t.Fatalf("AppendMessages duplicate seq: %v", err)
	}

	got, err := s.ListMessages(context.Background(), "ws1", task.ID, 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after duplicate insert, got %d: %#v", len(got), got)
	}
	if got[0].Seq != 1 || got[0].Type != "text" || got[0].Content != "hello" || got[0].SessionID != "sess-1" {
		t.Fatalf("unexpected first message: %#v", got[0])
	}
	if got[1].Seq != 2 || got[1].Tool != "exec_command" || got[1].CallID != "call-1" || string(got[1].Input) != `{"cmd":"git status"}` {
		t.Fatalf("unexpected second message: %#v", got[1])
	}

	sinceOne, err := s.ListMessages(context.Background(), "ws1", task.ID, 1)
	if err != nil {
		t.Fatalf("ListMessages since 1: %v", err)
	}
	if len(sinceOne) != 1 || sinceOne[0].Seq != 2 {
		t.Fatalf("expected only seq 2 after since=1, got %#v", sinceOne)
	}
}

func TestPinSessionBeforeCompletion(t *testing.T) {
	s, _, _ := newTestTaskService(t)
	task, err := s.CreateTaskForJob(context.Background(), "job-session", "ws1", "a1", "rt1", "src1", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob: %v", err)
	}

	if err := s.PinSession(context.Background(), "ws1", task.ID, "sess-early", "/tmp/mulwiki-job"); err != nil {
		t.Fatalf("PinSession: %v", err)
	}

	got, err := store.NewTaskStore(s.DB).GetInWorkspace(context.Background(), "ws1", task.ID)
	if err != nil {
		t.Fatalf("GetInWorkspace: %v", err)
	}
	if got.SessionID != "sess-early" || got.WorkDir != "/tmp/mulwiki-job" {
		t.Fatalf("expected pinned session/workdir, got session=%q workdir=%q", got.SessionID, got.WorkDir)
	}
}
