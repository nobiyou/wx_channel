package poc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

type PageAPI interface {
	Call(context.Context, string, any) ([]byte, error)
}

type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type Collector struct {
	api         PageAPI
	evidence    *EvidenceRecorder
	store       *Store
	clock       Clock
	lastRequest time.Time
	sequence    int
}

func NewCollector(api PageAPI, evidence *EvidenceRecorder, store *Store, clock Clock) *Collector {
	return &Collector{api: api, evidence: evidence, store: store, clock: clock}
}

func (c *Collector) call(ctx context.Context, method string, body any) ([]byte, SourceRef, error) {
	if c == nil || c.api == nil || c.evidence == nil || c.store == nil || c.clock == nil {
		return nil, SourceRef{}, errors.New("collector dependency is missing")
	}
	now := c.clock.Now()
	if c.sequence > 0 {
		wait := c.lastRequest.Add(time.Second).Sub(now)
		if wait > 0 {
			if err := c.clock.Sleep(ctx, wait); err != nil {
				return nil, SourceRef{}, err
			}
		}
	}
	c.lastRequest = c.clock.Now()
	raw, err := c.api.Call(ctx, method, body)
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

func (c *Collector) CollectWorks(ctx context.Context, options Options) ([]Work, CoverageStatus, error) {
	seenIDs := make(map[string]struct{})
	seenMarkers := make(map[string]struct{})
	works := make([]Work, 0, options.Limits.Works)
	marker := ""
	pageNumber := 0
	for len(works) < options.Limits.Works {
		pageNumber++
		raw, source, err := c.call(ctx, "finderSearch", map[string]any{"keyword": options.Keyword, "next_marker": marker})
		if err != nil {
			return works, CoverageIncomplete, err
		}
		items, nextMarker, markerPresent, err := parseSearchPage(raw)
		if err != nil {
			return works, CoverageIncomplete, err
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
				return works, CoverageTargetMet, nil
			}
		}
		if !markerPresent {
			return works, CoverageIncomplete, nil
		}
		if nextMarker == "" {
			return works, CoverageSourceExhausted, nil
		}
		if _, repeated := seenMarkers[nextMarker]; repeated {
			return works, CoverageIncomplete, nil
		}
		seenMarkers[nextMarker] = struct{}{}
		marker = nextMarker
	}
	return works, CoverageTargetMet, nil
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
