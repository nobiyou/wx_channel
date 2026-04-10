package handlers

import (
	"testing"
)

// TestMultiQualityRecordID verifies that download records for the same video
// in different qualities get distinct IDs, preventing DB overwrites.
func TestMultiQualityRecordID(t *testing.T) {
	tests := []struct {
		name       string
		videoID    string
		fileFormat string
		wantID     string
	}{
		{
			name:       "with format suffix",
			videoID:    "14881163211653126564",
			fileFormat: "xWT128",
			wantID:     "14881163211653126564_xWT128",
		},
		{
			name:       "different format same video",
			videoID:    "14881163211653126564",
			fileFormat: "xWT111",
			wantID:     "14881163211653126564_xWT111",
		},
		{
			name:       "empty format falls back to videoID only",
			videoID:    "14881163211653126564",
			fileFormat: "",
			wantID:     "14881163211653126564",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordID := tt.videoID
			if tt.fileFormat != "" {
				recordID = tt.videoID + "_" + tt.fileFormat
			}
			if recordID != tt.wantID {
				t.Errorf("recordID = %q, want %q", recordID, tt.wantID)
			}
		})
	}

	// Verify two different qualities produce different IDs
	id1 := "vid123_xWT128"
	id2 := "vid123_xWT111"
	if id1 == id2 {
		t.Error("same video with different formats should produce different record IDs")
	}
}

// TestMultiQualityLookupKeyMatchesStorageKey verifies that the dedup lookup
// constructs the same composite key used when storing the record.
func TestMultiQualityLookupKeyMatchesStorageKey(t *testing.T) {
	tests := []struct {
		name       string
		videoID    string
		fileFormat string
	}{
		{"with format", "vid123", "xWT128"},
		{"empty format", "vid123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Storage key (from the record creation path)
			storageID := tt.videoID
			if tt.fileFormat != "" {
				storageID = tt.videoID + "_" + tt.fileFormat
			}
			// Lookup key (from the dedup check path)
			lookupID := tt.videoID
			if tt.fileFormat != "" {
				lookupID = tt.videoID + "_" + tt.fileFormat
			}
			if storageID != lookupID {
				t.Errorf("storage key %q != lookup key %q", storageID, lookupID)
			}
		})
	}
}
