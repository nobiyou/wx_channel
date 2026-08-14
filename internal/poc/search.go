package poc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"
)

type PageAPI interface {
	Call(context.Context, string, any) ([]byte, error)
}

type CollectorStore interface {
	JobDir() string
	WriteEvidence(Evidence) (string, error)
	MaxEvidenceSequence() (int, error)
	SaveCheckpoint(Checkpoint) error
	LoadCheckpoint() (Checkpoint, error)
}

type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type Collector struct {
	api             PageAPI
	evidence        *EvidenceRecorder
	store           CollectorStore
	clock           Clock
	lastRequest     time.Time
	requestStarted  bool
	sequence        int
	sequenceReady   bool
	retryPolicy     RetryPolicy
	waiter          *WaitController
	readySignal     func(WaitReason, int) <-chan struct{}
	currentWorkRank int
}

func NewCollector(api PageAPI, evidence *EvidenceRecorder, store CollectorStore, clock Clock) *Collector {
	return &Collector{api: api, evidence: evidence, store: store, clock: clock, retryPolicy: DefaultRetryPolicy()}
}

func (c *Collector) ConfigureHumanWait(waiter *WaitController, readySignal func(WaitReason, int) <-chan struct{}) {
	if c == nil {
		return
	}
	c.waiter = waiter
	c.readySignal = readySignal
}

func (c *Collector) call(ctx context.Context, method string, body any) ([]byte, SourceRef, error) {
	if c == nil || c.api == nil || c.evidence == nil || c.store == nil || c.clock == nil {
		return nil, SourceRef{}, errors.New("collector dependency is missing")
	}
	if !c.sequenceReady {
		sequence, err := c.store.MaxEvidenceSequence()
		if err != nil {
			return nil, SourceRef{}, err
		}
		c.sequence = sequence
		c.sequenceReady = true
	}
	raw, err := c.callPage(ctx, method, body)
	if err != nil {
		return nil, SourceRef{}, err
	}
	c.sequence++
	evidence, err := c.evidence.Observe(c.sequence, method, raw)
	if err != nil {
		return nil, SourceRef{}, err
	}
	reference, err := c.store.WriteEvidence(evidence)
	if err != nil {
		return nil, SourceRef{}, err
	}
	return raw, SourceRef{Method: method, EvidenceRef: reference}, nil
}

func (c *Collector) callPage(ctx context.Context, method string, body any) ([]byte, error) {
	retries := 0
	targetContextRetried := false
	for {
		if c.requestStarted {
			wait := c.lastRequest.Add(time.Second).Sub(c.clock.Now())
			if wait > 0 {
				if err := c.clock.Sleep(ctx, wait); err != nil {
					return nil, err
				}
			}
		}
		c.lastRequest = c.clock.Now()
		c.requestStarted = true
		raw, err := c.api.Call(ctx, method, body)
		if err == nil {
			return raw, nil
		}
		category := ClassifyError(err)
		if category == ErrorTargetContext && c.waiter != nil && !targetContextRetried {
			var ready <-chan struct{}
			if c.readySignal != nil {
				ready = c.readySignal(WaitTargetContext, c.currentWorkRank)
			}
			result := c.waiter.Wait(ctx, WaitTargetContext, c.currentWorkRank, ready)
			if result != WaitResolved {
				return nil, HumanWaitError{Result: result}
			}
			targetContextRetried = true
			continue
		}
		if category != ErrorTransient || retries >= c.retryPolicy.MaxRetries || retries >= len(c.retryPolicy.Backoff) {
			return nil, CategorizedError{Category: category}
		}
		backoff := c.retryPolicy.Backoff[retries]
		retries++
		if err := c.clock.Sleep(ctx, backoff); err != nil {
			return nil, err
		}
	}
}

func (c *Collector) CollectWorks(ctx context.Context, options Options) ([]Work, CoverageStatus, error) {
	c.currentWorkRank = 0
	seenIDs := make(map[string]struct{})
	seenMarkers := make(map[string]struct{})
	works := make([]Work, 0, options.Limits.Works)
	marker := ""
	pageNumber := 0
	if checkpoint, ok, err := c.searchCheckpoint(); err != nil {
		return nil, CoverageIncomplete, err
	} else if ok {
		works = append(works, checkpoint.Works...)
		for _, work := range works {
			if work.WorkID != nil {
				seenIDs[*work.WorkID] = struct{}{}
			}
			if work.Locator.SearchPage > pageNumber {
				pageNumber = work.Locator.SearchPage
			}
		}
		marker = checkpoint.SearchMarker
		if marker != "" {
			seenMarkers[marker] = struct{}{}
		}
		if checkpoint.Phase == "search_complete" {
			if len(works) >= options.Limits.Works {
				return works[:options.Limits.Works], CoverageTargetMet, nil
			}
			return works, CoverageSourceExhausted, nil
		}
		if checkpoint.Phase == "search_incomplete" {
			return works, CoverageIncomplete, nil
		}
	}
	for len(works) < options.Limits.Works {
		pageNumber++
		raw, source, err := c.call(ctx, "finderSearch", map[string]any{"keyword": options.Keyword, "next_marker": marker})
		if err != nil {
			if checkpointErr := c.saveSearchCheckpoint("search", marker, works); checkpointErr != nil {
				return works, CoverageIncomplete, checkpointErr
			}
			return works, CoverageIncomplete, err
		}
		items, nextMarker, markerPresent, err := parseSearchPage(raw)
		if err != nil {
			if checkpointErr := c.saveSearchCheckpoint("search", marker, works); checkpointErr != nil {
				return works, CoverageIncomplete, checkpointErr
			}
			return works, CoverageIncomplete, CategorizedError{Category: ErrorStructure}
		}
		for index, item := range items {
			id, ok := stringField(item, "id")
			if !ok || id == "" {
				continue
			}
			if _, duplicate := seenIDs[id]; duplicate {
				continue
			}
			seenIDs[id] = struct{}{}
			rank := len(works) + 1
			itemSource := source
			itemSource.Ordinal = index + 1
			works = append(works, mapSearchWork(item, id, options.Keyword, rank, pageNumber, index+1, itemSource))
			if len(works) == options.Limits.Works {
				break
			}
		}
		if len(works) >= options.Limits.Works {
			if err := c.saveSearchCheckpoint("search_complete", nextMarker, works); err != nil {
				return works, CoverageIncomplete, err
			}
			return works, CoverageTargetMet, nil
		}
		if !markerPresent {
			if err := c.saveSearchCheckpoint("search_incomplete", "", works); err != nil {
				return works, CoverageIncomplete, err
			}
			return works, CoverageIncomplete, nil
		}
		if nextMarker == "" {
			if err := c.saveSearchCheckpoint("search_complete", "", works); err != nil {
				return works, CoverageIncomplete, err
			}
			return works, CoverageSourceExhausted, nil
		}
		if _, repeated := seenMarkers[nextMarker]; repeated {
			if err := c.saveSearchCheckpoint("search_incomplete", nextMarker, works); err != nil {
				return works, CoverageIncomplete, err
			}
			return works, CoverageIncomplete, nil
		}
		if err := c.saveSearchCheckpoint("search", nextMarker, works); err != nil {
			return works, CoverageIncomplete, err
		}
		seenMarkers[nextMarker] = struct{}{}
		marker = nextMarker
	}
	return works, CoverageTargetMet, nil
}

func (c *Collector) searchCheckpoint() (Checkpoint, bool, error) {
	checkpoint, err := c.store.LoadCheckpoint()
	if errors.Is(err, ErrCheckpointNotFound) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	if checkpoint.SchemaVersion != SchemaVersion || checkpoint.JobID != filepath.Base(c.store.JobDir()) {
		return Checkpoint{}, false, errors.New("checkpoint identity mismatch")
	}
	switch checkpoint.Phase {
	case "search", "search_complete", "search_incomplete":
		return checkpoint, true, nil
	default:
		return Checkpoint{}, false, nil
	}
}

func (c *Collector) saveSearchCheckpoint(phase, marker string, works []Work) error {
	return c.store.SaveCheckpoint(Checkpoint{
		SchemaVersion:   SchemaVersion,
		JobID:           filepath.Base(c.store.JobDir()),
		Phase:           phase,
		SearchMarker:    marker,
		CurrentWorkRank: len(works),
		Works:           append([]Work(nil), works...),
		SavedAt:         c.clock.Now().UTC(),
	})
}

func parseSearchPage(raw []byte) ([]map[string]any, string, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, "", false, errors.New("decode search response")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, "", false, errors.New("search response contains trailing JSON")
	}
	data, ok := root["data"].(map[string]any)
	if !ok {
		return nil, "", false, errors.New("search response data is missing")
	}
	container := data
	if nested, ok := data["data"].(map[string]any); ok {
		container = nested
	}
	rawItems, ok := container["objectList"].([]any)
	if !ok {
		return nil, "", false, errors.New("search object list is missing")
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		if item, ok := rawItem.(map[string]any); ok {
			items = append(items, item)
		} else {
			items = append(items, nil)
		}
	}
	markerValue, markerPresent := container["lastBuffer"]
	if !markerPresent {
		return items, "", false, nil
	}
	marker, ok := markerValue.(string)
	if !ok {
		return nil, "", false, errors.New("search marker is not a string")
	}
	return items, marker, true, nil
}

func mapSearchWork(item map[string]any, id, keyword string, rank, page, index int, source SourceRef) Work {
	work := Work{
		WorkID:           stringPointer(id),
		ObjectNonceID:    optionalString(item, "objectNonceId"),
		Author:           mapSearchAccount(item),
		MediaType:        mapSearchMedia(item),
		CollectionStatus: "pending",
		Locator: WorkLocator{
			Keyword: keyword, SearchRank: rank, SearchPage: page, IndexInPage: index,
		},
		Source: source,
	}
	if description, ok := item["objectDesc"].(map[string]any); ok {
		work.Title = optionalString(description, "description")
	}
	return work
}

func mapSearchAccount(item map[string]any) PublicAccount {
	account := PublicAccount{}
	if contact, ok := item["contact"].(map[string]any); ok {
		account.AccountID = optionalString(contact, "username")
		account.DisplayName = optionalString(contact, "nickname")
	}
	if account.AccountID == nil {
		account.AccountID = optionalString(item, "username")
	}
	if account.DisplayName == nil {
		account.DisplayName = optionalString(item, "nickname")
	}
	return account
}

func mapSearchMedia(item map[string]any) MediaType {
	media := MediaType{Normalized: "unknown"}
	if description, ok := item["objectDesc"].(map[string]any); ok {
		media.RawCode = description["mediaType"]
		switch fmt.Sprint(media.RawCode) {
		case "2", "4":
			media.Normalized = "video"
		}
	}
	return media
}

func optionalString(object map[string]any, key string) *string {
	value, ok := stringField(object, key)
	if !ok || value == "" {
		return nil
	}
	return stringPointer(value)
}

func stringField(object map[string]any, key string) (string, bool) {
	value, ok := object[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

func stringPointer(value string) *string { return &value }
