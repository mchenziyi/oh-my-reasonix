package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func pairedFixture(t *testing.T, n int) string {
	t.Helper()
	fx := PairedBenchmarkFixture{SchemaVersion: SchemaVersion, FixtureID: "paired_fixture"}
	for i := 0; i < n; i++ {
		fx.Cases = append(fx.Cases, PairedBenchmarkCase{CaseID: "case_" + string(rune('a'+i)), Mnemosyne: PairedBenchmarkArm{RetrievalHits: 2, RetrievalCandidates: 2, Reads: 2, Adoptions: 1, DownstreamSuccess: 1, DownstreamTotal: 1}, Native: PairedBenchmarkArm{RetrievalHits: 1, RetrievalCandidates: 2, Reads: 1, Adoptions: 0, DownstreamSuccess: 0, DownstreamTotal: 1}})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "paired.json")
	b, err := json.Marshal(fx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPairedBenchmarkReportsThreeLayerMetrics(t *testing.T) {
	rep, err := RunPairedBenchmarkFixture(context.Background(), pairedFixture(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.ProtocolOnly || rep.EvidenceStatus != "sufficient" || rep.Mnemosyne.RetrievalRecall != 1 || rep.Native.RetrievalRecall != .5 || rep.Mnemosyne.DownstreamSuccessRate != 1 {
		t.Fatalf("unexpected paired report: %+v", rep)
	}
}

func TestPairedBenchmarkInsufficientAndRejectsUnsafeInput(t *testing.T) {
	rep, err := RunPairedBenchmarkFixture(context.Background(), pairedFixture(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	if rep.EvidenceStatus != "insufficient_evidence" {
		t.Fatalf("want insufficient evidence: %+v", rep)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"fixture_id":"paired_bad","cases":[{"case_id":"x","mnemosyne":{"retrieval_hits":-1}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunPairedBenchmarkFixture(context.Background(), path); err == nil {
		t.Fatal("negative counts must fail closed")
	}
}

func TestPairedBenchmarkIsByteStable(t *testing.T) {
	path := pairedFixture(t, 3)
	a, err := RunPairedBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunPairedBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ba, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ba) != string(bb) {
		t.Fatal("paired report must be byte stable")
	}
}
