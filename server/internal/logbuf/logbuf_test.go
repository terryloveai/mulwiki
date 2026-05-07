package logbuf

import (
	"testing"
)

func TestAppendAndSince(t *testing.T) {
	s := NewStore(100)

	s.Append("job1", "stdout", "line 1")
	s.Append("job1", "stdout", "line 2")
	s.Append("job1", "stderr", "error 1")

	entries, next := s.Since("job1", 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if next != 3 {
		t.Fatalf("expected next=3, got %d", next)
	}
	if entries[0].Line != "line 1" {
		t.Errorf("entries[0] = %q, want %q", entries[0].Line, "line 1")
	}
	if entries[1].Stream != "stdout" {
		t.Errorf("entries[1].Stream = %q, want %q", entries[1].Stream, "stdout")
	}
	if entries[2].Stream != "stderr" {
		t.Errorf("entries[2].Stream = %q, want %q", entries[2].Stream, "stderr")
	}

	// Since from an existing cursor should return only new entries.
	entries2, next2 := s.Since("job1", 2)
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry since 2, got %d", len(entries2))
	}
	if next2 != 3 {
		t.Fatalf("expected next=3, got %d", next2)
	}
}

func TestRingBuffer(t *testing.T) {
	s := NewStore(5)

	for i := 0; i < 7; i++ {
		s.Append("job1", "stdout", "line")
	}

	total := s.Total("job1")
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}

	// The ring buffer wraps, so we should get 5 entries.
	entries, next := s.Since("job1", 0)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	if next != 5 {
		t.Fatalf("expected next=5, got %d", next)
	}
}

func TestRemove(t *testing.T) {
	s := NewStore(100)
	s.Append("job1", "stdout", "line")
	s.Remove("job1")

	entries, _ := s.Since("job1", 0)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after remove, got %d", len(entries))
	}

	total := s.Total("job1")
	if total != 0 {
		t.Fatalf("expected total=0 after remove, got %d", total)
	}
}

func TestSinceEmpty(t *testing.T) {
	s := NewStore(100)
	entries, next := s.Since("nonexistent", 0)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
	if next != 0 {
		t.Fatalf("expected next=0, got %d", next)
	}
}

func TestSincePastEnd(t *testing.T) {
	s := NewStore(100)
	s.Append("job1", "stdout", "line")

	entries, next := s.Since("job1", 5)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
	if next != 1 {
		t.Fatalf("expected next=1, got %d", next)
	}
}
