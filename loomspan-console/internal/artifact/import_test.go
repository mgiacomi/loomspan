package artifact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

func newImportTestService(t *testing.T, config Config, processor *fakeProcessor) *Service {
	t.Helper()
	return newTestServiceWithProcessor(t, config,
		newFakeLoader(testTraceMetadata("unused", 0)), newFakeOpener(nil, 0),
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1_000_000)), nil, processor)
}

func TestImportPublishesImportedEvidenceWithProcessorMetadata(t *testing.T) {
	processor := newFakeProcessor()
	processor.metadata = &TraceMetadata{TraceID: "trace-import", SessionID: "session-import", EntrySkill: "validated.entry", Outcome: "SUCCEEDED", FinalizedAt: time.Unix(42, 0).UTC()}
	svc := newImportTestService(t, Config{Unlimited: true, NeverExpire: true}, processor)

	got, domain := svc.Import(context.Background(), bytes.NewReader([]byte("raw-import")), -1)
	if domain != nil {
		t.Fatalf("Import failed: %v", domain)
	}
	if got.Owner.Source() != evidence.SourceImported || got.Metadata.TraceID != "trace-import" || got.Metadata.SessionID != "session-import" || got.Metadata.EntrySkill != "validated.entry" {
		t.Fatalf("unexpected imported artifact: %+v", got)
	}
	lookup, domain := svc.Lookup(evidence.ForImported(), "trace-import")
	if domain != nil || !lookup.LocalAvailable || lookup.Handle != got.Handle {
		t.Fatalf("imported lookup=%+v error=%v", lookup, domain)
	}
	if lookup.Metadata.EntrySkill != "validated.entry" {
		t.Fatalf("validated entry skill was not published: %+v", lookup.Metadata)
	}
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil || len(snapshot.Entries) != 1 || snapshot.Entries[0].Source != evidence.SourceImported || snapshot.Entries[0].TargetScopeID != "" || snapshot.Entries[0].ApplicationTraceExpiresAt != nil {
		t.Fatalf("unexpected imported snapshot=%+v error=%v", snapshot, domain)
	}
}

func TestImportRejectsDuplicateBeforeStagingOrCapacityWork(t *testing.T) {
	processor := newFakeProcessor()
	svc := newImportTestService(t, Config{Unlimited: true, NeverExpire: true}, processor)
	if _, domain := svc.Import(context.Background(), bytes.NewReader([]byte("first")), 5); domain != nil {
		t.Fatalf("first import failed: %v", domain)
	}
	before, _ := svc.StorageSnapshot()
	_, domain := svc.Import(context.Background(), bytes.NewReader([]byte("second")), 6)
	if domain == nil || domain.Code != consolecore.CodeArtifactAlreadyExists {
		t.Fatalf("expected ARTIFACT_ALREADY_EXISTS, got %v", domain)
	}
	after, _ := svc.StorageSnapshot()
	if after.AcquiredCount != 1 || after.ChargedBytes != before.ChargedBytes {
		t.Fatalf("duplicate changed storage: before=%+v after=%+v", before, after)
	}
}

func TestImportTimestampIsRenewedOnlyAfterRemovalAndReimport(t *testing.T) {
	processor := newFakeProcessor()
	clock := newManualClock(time.UnixMilli(1_000_000))
	svc := newTestServiceWithProcessor(t, Config{Unlimited: true, NeverExpire: true},
		newFakeLoader(testTraceMetadata("unused", 0)), newFakeOpener(nil, 0),
		&manualTimerFactory{}, clock, nil, processor)

	first, domain := svc.Import(context.Background(), bytes.NewReader([]byte("first")), -1)
	if domain != nil {
		t.Fatalf("first import failed: %v", domain)
	}
	clock.advance(time.Minute)
	if _, domain = svc.Import(context.Background(), bytes.NewReader([]byte("duplicate")), -1); domain == nil || domain.Code != consolecore.CodeArtifactAlreadyExists {
		t.Fatalf("duplicate import domain=%v", domain)
	}
	lookup, domain := svc.Lookup(evidence.ForImported(), first.Metadata.TraceID)
	if domain != nil || !lookup.AcquiredAt.Equal(first.AcquiredAt) {
		t.Fatalf("duplicate import changed imported timestamp: lookup=%+v domain=%v", lookup, domain)
	}
	if domain = svc.Remove(evidence.ForImported(), first.Metadata.TraceID); domain != nil {
		t.Fatalf("remove failed: %v", domain)
	}
	clock.advance(time.Minute)
	second, domain := svc.Import(context.Background(), bytes.NewReader([]byte("second")), -1)
	if domain != nil || !second.AcquiredAt.After(first.AcquiredAt) {
		t.Fatalf("reimport did not renew imported timestamp: first=%+v second=%+v domain=%v", first, second, domain)
	}
}

func TestImportDeclaredLengthLimitRejectsWithoutReading(t *testing.T) {
	processor := newFakeProcessor()
	processor.derivedBytes = nil
	svc := newImportTestService(t, Config{MaxBytes: 3, NeverExpire: true}, processor)
	reader := &countingReader{data: []byte("four")}
	_, domain := svc.Import(context.Background(), reader, 4)
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded || reader.reads != 0 {
		t.Fatalf("expected unread LIMIT_EXCEEDED, reads=%d error=%v", reader.reads, domain)
	}
}

func TestImportAcceptsKnownAndUnknownLengthsAtEffectiveLimit(t *testing.T) {
	for _, declared := range []int64{3, -1} {
		t.Run(fmt.Sprintf("declared-%d", declared), func(t *testing.T) {
			processor := newFakeProcessor()
			processor.derivedBytes = nil
			svc := newImportTestService(t, Config{MaxBytes: 3, NeverExpire: true}, processor)
			if _, domain := svc.Import(context.Background(), bytes.NewReader([]byte("raw")), declared); domain != nil {
				t.Fatalf("Import length %d failed: %v", declared, domain)
			}
		})
	}
}

func TestImportRejectsCanceledRequestBeforeReading(t *testing.T) {
	processor := newFakeProcessor()
	svc := newImportTestService(t, Config{Unlimited: true, NeverExpire: true}, processor)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &countingReader{data: []byte("unused")}

	_, domain := svc.Import(ctx, reader, -1)
	if domain == nil || domain.Code != consolecore.CodeConsoleError || reader.reads != 0 {
		t.Fatalf("expected unread canceled import, reads=%d error=%v", reader.reads, domain)
	}
}

func TestImportedEvidenceSurvivesTargetScopeRotation(t *testing.T) {
	processor := newFakeProcessor()
	svc := newImportTestService(t, Config{Unlimited: true, NeverExpire: true}, processor)
	if _, domain := svc.Import(context.Background(), bytes.NewReader([]byte("imported")), -1); domain != nil {
		t.Fatalf("Import failed: %v", domain)
	}
	oldScope, cancelOld := testScope("scope-old")
	defer cancelOld()
	svc.ActivateActivity(oldScope)
	newScope, cancelNew := testScope("scope-new")
	defer cancelNew()
	svc.ActivateActivity(newScope)
	svc.InvalidateTargetScope(oldScope.ID, oldScope.Context)

	lookup, domain := svc.Lookup(evidence.ForImported(), "trace-import")
	if domain != nil || !lookup.LocalAvailable {
		t.Fatalf("imported evidence did not survive target rotation: lookup=%+v error=%v", lookup, domain)
	}
}

func TestConcurrentDuplicateImportsPublishExactlyOneWinner(t *testing.T) {
	processor := newFakeProcessor()
	svc := newImportTestService(t, Config{Unlimited: true, NeverExpire: true}, processor)
	const contenders = 20
	start := make(chan struct{})
	results := make(chan *consolecore.Error, contenders)
	var wait sync.WaitGroup
	wait.Add(contenders)
	for index := 0; index < contenders; index++ {
		go func() {
			defer wait.Done()
			<-start
			_, domain := svc.Import(context.Background(), bytes.NewReader([]byte("same-trace")), -1)
			results <- domain
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	winners := 0
	duplicates := 0
	for domain := range results {
		if domain == nil {
			winners++
		} else if domain.Code == consolecore.CodeArtifactAlreadyExists {
			duplicates++
		} else {
			t.Fatalf("unexpected concurrent import error: %v", domain)
		}
	}
	if winners != 1 || duplicates != contenders-1 {
		t.Fatalf("winners=%d duplicates=%d", winners, duplicates)
	}
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil || snapshot.AcquiredCount != 1 {
		t.Fatalf("unexpected storage after duplicate race: snapshot=%+v error=%v", snapshot, domain)
	}
}

type countingReader struct {
	data   []byte
	reads  int
	offset int
}

func (reader *countingReader) Read(p []byte) (int, error) {
	reader.reads++
	if reader.offset >= len(reader.data) {
		return 0, io.EOF
	}
	n := copy(p, reader.data[reader.offset:])
	reader.offset += n
	return n, nil
}
