package poc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixturePageAPI struct {
	responses [][]byte
	calls     int
}

func (a *fixturePageAPI) Call(context.Context, string, any) ([]byte, error) {
	if a.calls >= len(a.responses) {
		return nil, fmt.Errorf("unexpected API call %d", a.calls+1)
	}
	response := a.responses[a.calls]
	a.calls++
	return response, nil
}

type fixtureClock struct{ now time.Time }

func (c *fixtureClock) Now() time.Time { return c.now }
func (c *fixtureClock) Sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		c.now = c.now.Add(duration)
		return nil
	}
}

func TestCollectSearchPreservesOrderAndExhaustion(t *testing.T) {
	api := &fixturePageAPI{responses: [][]byte{readFixture(t, "search-page-1.json"), readFixture(t, "search-page-end.json")}}
	clock := &fixtureClock{}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "search-job"), clock)
	works, coverage, err := collector.CollectWorks(context.Background(), approvedTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 3 || works[0].Locator.SearchRank != 1 || works[2].Locator.SearchRank != 3 {
		t.Fatalf("works=%+v", works)
	}
	if works[2].MediaType.Normalized != "unknown" {
		t.Fatalf("non-video media=%+v", works[2].MediaType)
	}
	if coverage != CoverageSourceExhausted {
		t.Fatalf("coverage=%s", coverage)
	}
	if clock.now != (time.Time{}).Add(time.Second) {
		t.Fatalf("request spacing=%s", clock.now)
	}
}

func TestEmptyPageWithoutExplicitLastBufferIsNotExhaustion(t *testing.T) {
	api := &fixturePageAPI{responses: [][]byte{[]byte(`{"data":{"objectList":[]}}`)}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "empty-job"), &fixtureClock{})
	_, coverage, err := collector.CollectWorks(context.Background(), approvedTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if coverage != CoverageIncomplete {
		t.Fatalf("coverage=%s", coverage)
	}
}

func TestRepeatedSearchMarkerStopsAsIncomplete(t *testing.T) {
	page := []byte(`{"data":{"objectList":[],"lastBuffer":"repeat"}}`)
	api := &fixturePageAPI{responses: [][]byte{page, page}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "repeat-job"), &fixtureClock{})
	_, coverage, err := collector.CollectWorks(context.Background(), approvedTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if coverage != CoverageIncomplete || api.calls != 2 {
		t.Fatalf("coverage=%s calls=%d", coverage, api.calls)
	}
}

func TestSearchStopsAtTenUniqueValidWorks(t *testing.T) {
	items := ""
	for i := 1; i <= 12; i++ {
		if i > 1 {
			items += ","
		}
		items += fmt.Sprintf(`{"id":"fixture-work-%d","objectDesc":{"mediaType":2}}`, i)
	}
	page := []byte(`{"data":{"objectList":[` + items + `],"lastBuffer":"more"}}`)
	api := &fixturePageAPI{responses: [][]byte{page}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "limit-job"), &fixtureClock{})
	works, coverage, err := collector.CollectWorks(context.Background(), approvedTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 10 || coverage != CoverageTargetMet || api.calls != 1 {
		t.Fatalf("works=%d coverage=%s calls=%d", len(works), coverage, api.calls)
	}
}

func approvedTestOptions() Options {
	options := DefaultOptions()
	options.AckIsolatedVM = true
	return options
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
