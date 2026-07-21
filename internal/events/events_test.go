package events

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestWriterWritesJSONL(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	writer.clock = func() time.Time { return time.Unix(0, 0) }
	if err := writer.Write("omr.fixture.completed", "demo", "passed", nil); err != nil {
		t.Fatal(err)
	}
	var record Event
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Event != "omr.fixture.completed" || record.FixtureID != "demo" || record.Status != "passed" {
		t.Fatalf("unexpected event: %+v", record)
	}
}

func TestWriterIncludesErrors(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	if err := writer.Write("omr.fixture.failed", "demo", "failed", errSentinel{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"error":"sentinel"`)) {
		t.Fatalf("error not written: %s", output.Bytes())
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "sentinel" }
