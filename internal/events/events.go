package events

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Event is the stable OMR lifecycle record written as JSONL.
type Event struct {
	Time      string `json:"time"`
	Event     string `json:"event"`
	FixtureID string `json:"fixture_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Writer struct {
	encoder *json.Encoder
	clock   func() time.Time
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{encoder: json.NewEncoder(w), clock: time.Now}
}

func (w *Writer) Write(event, fixtureID, status string, runErr error) error {
	if w == nil || w.encoder == nil {
		return fmt.Errorf("event writer is nil")
	}
	record := Event{Time: w.clock().UTC().Format(time.RFC3339Nano), Event: event, FixtureID: fixtureID, Status: status}
	if runErr != nil {
		record.Error = runErr.Error()
	}
	return w.encoder.Encode(record)
}
