package sync

import (
	"fmt"
	"testing"

	"github.com/Gentleman-Programming/engram/v3/internal/store"
)

// importReadAmplificationObservationChunk returns a minimal chunk that carries
// one observation upsert, so a manifest can be filled with known entries that
// hold no relation and therefore no dependency evidence.
func importReadAmplificationObservationChunk(t *testing.T, index int) ChunkData {
	t.Helper()
	sessionID := fmt.Sprintf("sess-amp-%03d", index)
	obsID := fmt.Sprintf("obs-amp-%03d", index)
	return ChunkData{Mutations: []store.SyncMutation{
		{
			Entity:    store.SyncEntitySession,
			EntityKey: sessionID,
			Op:        store.SyncOpUpsert,
			Payload:   fmt.Sprintf(`{"id":%q,"project":"proj-a","directory":"/tmp/proj-a"}`, sessionID),
		},
		{
			Entity:    store.SyncEntityObservation,
			EntityKey: obsID,
			Op:        store.SyncOpUpsert,
			Payload:   fmt.Sprintf(`{"sync_id":%q,"session_id":%q,"type":"decision","title":"t","content":"c","project":"proj-a","scope":"project"}`, obsID, sessionID),
		},
	}}
}

// TestCloudImportRemoteReadsScaleWithManifest measures how many chunk reads a
// cloud import performs against the transport, and how that count changes when
// the pending set carries a relation whose endpoint is not known locally.
//
// Issue #1961 reports 11,134 GET /sync/pull requests for an import that
// applied 377 pending chunks, and attributes the cost to the relation
// dependency oracle: deciding whether a relation endpoint is permanently
// missing needs delete evidence for the whole manifest snapshot, and a known
// entry is read back from the transport to collect it. The issue could not
// confirm that with a profiler.
//
// This test is that confirmation, without a profiler: it counts the transport
// reads for a manifest of already-imported entries plus a small pending set,
// with and without a relation in the pending chunk. The difference between
// the two is the amplification the issue reports.
//
// The assertion pins the amplification rather than an exact count, so it does
// not freeze the implementation. A fix that stops reading known chunks will
// fail this test, and that failure is the signal to re-point the measurement
// at the fixed bound.
func TestCloudImportRemoteReadsScaleWithManifest(t *testing.T) {
	// The manifest sizes bracket the shape the issue reports: the cost has to
	// follow the manifest, not the pending set.
	manifestSizes := []int{60, 240}

	// The contrast case: a pending chunk with no relation needs no dependency
	// evidence, so the import reads only what it applies.
	plainReads := measureCloudImportReads(t, 60, false)
	t.Logf("pending chunk without a relation: known entries=60 pending=1 transport reads=%d", plainReads)
	if plainReads > 4 {
		t.Errorf("import read %d chunks for 1 pending chunk without a relation, want the pending chunk only", plainReads)
	}

	amplified := make(map[int]int, len(manifestSizes))
	for _, size := range manifestSizes {
		reads := measureCloudImportReads(t, size, true)
		amplified[size] = reads
		t.Logf("pending chunk with an unresolved relation endpoint: known entries=%d pending=1 transport reads=%d", size, reads)
		if reads < size {
			t.Errorf("import read %d chunks for 1 pending chunk, want at least %d: the relation dependency oracle is not reading the whole manifest", reads, size)
		}
	}

	// The cost has to follow the manifest size, which is the claim the issue
	// makes: an import that applies a handful of chunks still pays for the
	// whole snapshot.
	small, large := manifestSizes[0], manifestSizes[len(manifestSizes)-1]
	if amplified[large] <= amplified[small] {
		t.Errorf("transport reads did not grow with the manifest: %d entries -> %d reads, %d entries -> %d reads",
			small, amplified[small], large, amplified[large])
	}
}

// measureCloudImportReads imports a manifest of known entries plus one pending
// chunk and returns the number of chunk reads the import performed against
// the transport.
func measureCloudImportReads(t *testing.T, knownChunks int, pendingRelation bool) int {
	t.Helper()

	s := newTestStore(t)
	if err := s.EnrollProject("proj-a"); err != nil {
		t.Fatalf("enroll project: %v", err)
	}
	transport := newFakeCloudTransport()

	// The manifest entries that are already imported locally. They hold no
	// relation, so they contribute no dependency evidence.
	entries := make([]ChunkEntry, 0, knownChunks+1)
	for i := 0; i < knownChunks; i++ {
		id := fmt.Sprintf("chunk-amp-%03d", i)
		entries = append(entries, ChunkEntry{ID: id, CreatedAt: "2026-08-26T00:00:00Z"})
		transport.chunks[id] = mustJSONChunk(t, id, importReadAmplificationObservationChunk(t, i))
	}
	transport.manifest = &Manifest{Version: 1, Chunks: entries}

	// The first import applies the manifest and marks every entry as synced,
	// which is what makes the entries "known" to the next run.
	if _, err := NewCloudWithTransport(s, transport, "proj-a").Import(); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	// The pending chunk is the only new entry. It carries a relation whose
	// target endpoint is not known locally, which is the case that makes the
	// import ask for delete evidence.
	pendingID := "chunk-amp-pending"
	pending := importReadAmplificationObservationChunk(t, 999)
	if pendingRelation {
		pending = relationMissingEndpointChunk("sess-amp-pending", "obs-amp-pending-src", "obs-amp-pending-gone", "rel-amp-pending")
	}
	entries = append(entries, ChunkEntry{ID: pendingID, CreatedAt: "2026-08-27T00:00:00Z"})
	transport.chunks[pendingID] = mustJSONChunk(t, pendingID, pending)
	transport.manifest = &Manifest{Version: 1, Chunks: entries}

	transport.readChunkCalls = 0
	result, err := NewCloudWithTransport(s, transport, "proj-a").Import()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.ChunksImported != 1 {
		t.Fatalf("imported %d chunks, want 1 pending chunk", result.ChunksImported)
	}
	return transport.readChunkCalls
}
