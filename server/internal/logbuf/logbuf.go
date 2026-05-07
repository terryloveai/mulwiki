// Package logbuf provides a thread-safe in-memory ring buffer for job log lines.
// Each job gets its own buffer, trimmed to a maximum number of lines.
// The SSE endpoint reads from here, and the daemon pushes log lines here via HTTP.
package logbuf

import (
	"sync"
	"time"
)

// LogEntry is a single log line with metadata.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Line      string    `json:"line"`
}

// Buffer holds log entries for a single job.
type Buffer struct {
	mu       sync.RWMutex
	entries  []LogEntry
	maxLines int
	cursor   int // write cursor for ring buffer
	full     bool
}

// Store is the global log buffer registry.
type Store struct {
	mu      sync.RWMutex
	buffers map[string]*Buffer // jobID -> Buffer
	maxSize int
}

// NewStore creates a new log buffer store with the given per-job max line count.
func NewStore(maxLinesPerJob int) *Store {
	return &Store{
		buffers: make(map[string]*Buffer),
		maxSize: maxLinesPerJob,
	}
}

// Append adds a log entry to the job's buffer.
// If the job doesn't have a buffer yet, one is created.
func (s *Store) Append(jobID string, stream, line string) {
	s.mu.RLock()
	buf, ok := s.buffers[jobID]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		// Double-check after acquiring write lock.
		buf, ok = s.buffers[jobID]
		if !ok {
			buf = &Buffer{
				entries:  make([]LogEntry, s.maxSize),
				maxLines: s.maxSize,
			}
			s.buffers[jobID] = buf
		}
		s.mu.Unlock()
	}

	buf.mu.Lock()
	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Stream:    stream,
		Line:      line,
	}
	if buf.cursor < buf.maxLines {
		buf.entries[buf.cursor] = entry
		buf.cursor++
		if buf.cursor == buf.maxLines {
			buf.full = true
		}
	} else {
		// Ring buffer wrap.
		buf.cursor = 0
		buf.entries[0] = entry
		buf.cursor = 1
		buf.full = true
	}
	buf.mu.Unlock()
}

// Since returns all log entries for a job that have index >= sinceIndex.
// Returns (entries, nextIndex) where nextIndex can be used as sinceIndex in the next call.
func (s *Store) Since(jobID string, sinceIndex int) ([]LogEntry, int) {
	s.mu.RLock()
	buf, ok := s.buffers[jobID]
	s.mu.RUnlock()

	if !ok {
		return nil, 0
	}

	buf.mu.RLock()
	defer buf.mu.RUnlock()

	total := buf.cursor
	if buf.full {
		total = buf.maxLines
	}

	if sinceIndex >= total {
		return nil, total
	}

	// Collect entries from sinceIndex to current.
	result := make([]LogEntry, 0, total-sinceIndex)
	for i := sinceIndex; i < total; i++ {
		idx := i
		if buf.full {
			// In ring buffer mode, the "oldest" entry is at cursor,
			// and we need to read from (cursor + i) % maxLines.
			idx = (buf.cursor + i) % buf.maxLines
		}
		result = append(result, buf.entries[idx])
	}
	return result, total
}

// Total returns the total number of log entries for a job.
func (s *Store) Total(jobID string) int {
	s.mu.RLock()
	buf, ok := s.buffers[jobID]
	s.mu.RUnlock()

	if !ok {
		return 0
	}

	buf.mu.RLock()
	defer buf.mu.RUnlock()

	if buf.full {
		return buf.maxLines
	}
	return buf.cursor
}

// Remove deletes the log buffer for a job.
func (s *Store) Remove(jobID string) {
	s.mu.Lock()
	delete(s.buffers, jobID)
	s.mu.Unlock()
}
