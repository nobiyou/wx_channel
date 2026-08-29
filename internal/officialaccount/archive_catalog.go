package officialaccount

import (
	"strings"
)

// EnsureArticleArchiveRecord creates the catalog row needed by the archive
// adapter. Existing rows retain their archive state and metric history.
func (s *Service) EnsureArticleArchiveRecord(plan ArchivePlan) error {
	if s == nil {
		return ErrCatalogUnavailable
	}
	repository := s.catalogRepository()
	if repository == nil {
		return nil
	}
	if strings.TrimSpace(plan.Content.Biz) == "" {
		return ErrMissingBiz
	}
	if strings.TrimSpace(plan.Content.Key) == "" {
		return ErrArticleIdentity
	}
	record := ArticleRecord{
		Key:                    strings.TrimSpace(plan.Content.Key),
		Biz:                    strings.TrimSpace(plan.Content.Biz),
		Mid:                    strings.TrimSpace(plan.Content.Mid),
		Idx:                    plan.Content.Idx,
		FileID:                 plan.Content.FileID,
		VideoID:                strings.TrimSpace(plan.Content.VideoID),
		Title:                  strings.TrimSpace(plan.Content.Title),
		Digest:                 strings.TrimSpace(plan.Content.Description),
		Author:                 strings.TrimSpace(plan.Content.Author),
		ContentURL:             SanitizeArchiveMetadataURL(plan.Content.URL),
		SourceURL:              SanitizeArchiveMetadataURL(plan.Content.SourceURL),
		CoverURL:               SanitizeArchiveMetadataURL(plan.Content.CoverURL),
		PublishTime:            plan.Content.PublishTime,
		IsMulti:                plan.Content.IsMulti,
		IsOriginal:             plan.Content.IsOriginal,
		IsPaid:                 plan.Content.IsPaid,
		IsPaySubscribe:         plan.Content.IsPaySubscribe,
		ItemShowType:           plan.Content.ItemShowType,
		Subtype:                plan.Content.Subtype,
		CopyrightStat:          plan.Content.CopyrightStat,
		Duration:               plan.Content.Duration,
		AudioFileID:            plan.Content.AudioFileID,
		PlayURL:                SanitizeCatalogMediaURL(plan.Content.PlayURL),
		MaliciousTitleReasonID: plan.Content.MaliciousTitleReasonID,
		MaliciousContentType:   plan.Content.MaliciousContentType,
		ArchiveStatus:          ArchiveStatusQueued,
	}
	if record.SourceURL == "" {
		record.SourceURL = record.ContentURL
	}
	_, err := repository.UpsertArticles(record.Biz, []ArticleRecord{record}, s.now())
	return err
}

// UpdateArticleArchive writes the file-side outcome into the durable catalog.
// The no-op when no catalog is configured preserves the legacy archive-only
// service construction used by older callers and tests.
func (s *Service) UpdateArticleArchive(state ArticleArchiveState) error {
	if s == nil {
		return ErrCatalogUnavailable
	}
	repository := s.catalogRepository()
	if repository == nil {
		return nil
	}
	return repository.UpdateArticleArchive(state)
}
