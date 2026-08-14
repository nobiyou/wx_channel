package poc

import "time"

const SchemaVersion = "wx-channel-comment-poc/1.0"

type JobStatus string

const (
	JobCompleted     JobStatus = "completed"
	JobRequiresHuman JobStatus = "requires_human"
	JobPartial       JobStatus = "partial"
	JobFailed        JobStatus = "failed"
)

type CapabilityStatus string

const (
	CapabilityVerified         CapabilityStatus = "verified"
	CapabilityVerifiedWithGaps CapabilityStatus = "verified_with_gaps"
	CapabilityInconclusive     CapabilityStatus = "inconclusive"
	CapabilityFailed           CapabilityStatus = "failed"
)

type CoverageStatus string

const (
	CoverageTargetMet       CoverageStatus = "target_met"
	CoverageSourceExhausted CoverageStatus = "source_exhausted_below_target"
	CoverageIncomplete      CoverageStatus = "incomplete"
)

type FieldStatus string

const (
	FieldPresent           FieldStatus = "present"
	FieldMissingInSource   FieldStatus = "missing_in_source"
	FieldInvalidFormat     FieldStatus = "invalid_format"
	FieldNotApplicable     FieldStatus = "not_applicable"
	FieldRedactedForSafety FieldStatus = "redacted_for_safety"
)

type Limits struct {
	Works                   int `json:"works"`
	TopLevelCommentsPerWork int `json:"top_level_comments_per_work"`
	RepliesPerComment       int `json:"replies_per_comment"`
	RepliesPerWork          int `json:"replies_per_work"`
}

type HumanWaitPolicy struct {
	Timeout       time.Duration `json:"-"`
	Extension     time.Duration `json:"-"`
	MaxExtensions int           `json:"max_extensions_per_event"`
}

type Job struct {
	JobID            string           `json:"job_id"`
	Keyword          string           `json:"keyword"`
	Status           JobStatus        `json:"status"`
	CapabilityStatus CapabilityStatus `json:"capability_status"`
	CoverageStatus   CoverageStatus   `json:"coverage_status"`
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at"`
	Limits           Limits           `json:"limits"`
}

type MediaType struct {
	RawCode    any    `json:"raw_code"`
	Normalized string `json:"normalized"`
}

type SourceRef struct {
	Method      string `json:"method"`
	EvidenceRef string `json:"evidence_ref"`
	Ordinal     int    `json:"ordinal"`
}

type WorkLocator struct {
	Keyword     string  `json:"keyword"`
	SearchRank  int     `json:"search_rank"`
	SearchPage  int     `json:"search_page"`
	IndexInPage int     `json:"index_in_page"`
	PublicURL   *string `json:"public_url"`
}

type PublicAccount struct {
	AccountID   *string `json:"account_id"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type Work struct {
	WorkID               *string       `json:"work_id"`
	ObjectNonceID        *string       `json:"object_nonce_id"`
	Title                *string       `json:"title"`
	Author               PublicAccount `json:"author"`
	MediaType            MediaType     `json:"media_type"`
	Locator              WorkLocator   `json:"locator"`
	CollectionStatus     string        `json:"collection_status"`
	TopLevelCommentCount int           `json:"top_level_comment_count"`
	ReplyCount           int           `json:"reply_count"`
	Truncation           struct {
		Truncated bool     `json:"truncated"`
		Reasons   []string `json:"reasons"`
	} `json:"truncation"`
	Source SourceRef `json:"source"`
}

type CommentContent struct {
	Text      *string   `json:"text"`
	MediaType MediaType `json:"media_type"`
}

type CommentTime struct {
	Raw         *string `json:"raw"`
	UnixSeconds *int64  `json:"unix_seconds"`
	ISO8601     *string `json:"iso_8601"`
}

type IPLocation struct {
	Label *string `json:"label"`
}

type Comment struct {
	CommentID              *string        `json:"comment_id"`
	WorkID                 *string        `json:"work_id"`
	Level                  int            `json:"level"`
	ParentCommentID        *string        `json:"parent_comment_id"`
	RootCommentID          *string        `json:"root_comment_id"`
	RetrievalRootCommentID *string        `json:"retrieval_root_comment_id"`
	Content                CommentContent `json:"content"`
	Account                PublicAccount  `json:"account"`
	CreatedAt              CommentTime    `json:"created_at"`
	IPLocation             IPLocation     `json:"ip_location"`
	Source                 SourceRef      `json:"source"`
}

type Dataset struct {
	SchemaVersion string    `json:"schema_version"`
	Job           Job       `json:"job"`
	Works         []Work    `json:"works"`
	Comments      []Comment `json:"comments"`
}

type FieldResult struct {
	Path       string      `json:"path"`
	Status     FieldStatus `json:"status"`
	Applicable int         `json:"applicable"`
	Present    int         `json:"present"`
	ReasonCode string      `json:"reason_code"`
}

type Validation struct {
	JobID            string           `json:"job_id"`
	CapabilityStatus CapabilityStatus `json:"capability_status"`
	CoverageStatus   CoverageStatus   `json:"coverage_status"`
	Fields           []FieldResult    `json:"fields"`
	ReasonCodes      []string         `json:"reason_codes"`
}

type Counts struct {
	Works            int `json:"works"`
	TopLevelComments int `json:"top_level_comments"`
	Replies          int `json:"replies"`
}

type Manifest struct {
	SchemaVersion    string           `json:"schema_version"`
	JobID            string           `json:"job_id"`
	Status           JobStatus        `json:"status"`
	CapabilityStatus CapabilityStatus `json:"capability_status"`
	CoverageStatus   CoverageStatus   `json:"coverage_status"`
	Counts           Counts           `json:"counts"`
	CleanupSuccess   bool             `json:"cleanup_success"`
	CompletedAt      *time.Time       `json:"completed_at"`
	ReasonCodes      []string         `json:"reason_codes"`
}

type Checkpoint struct {
	SchemaVersion       string    `json:"schema_version"`
	JobID               string    `json:"job_id"`
	Phase               string    `json:"phase"`
	SearchMarker        string    `json:"search_marker"`
	CurrentWorkRank     int       `json:"current_work_rank"`
	PendingReplyRootIDs []string  `json:"pending_reply_root_ids"`
	CurrentReplyRootID  *string   `json:"current_reply_root_id"`
	Works               []Work    `json:"works"`
	Comments            []Comment `json:"comments"`
	SavedAt             time.Time `json:"saved_at"`
}

type CleanupReceipt struct {
	JobID       string          `json:"job_id"`
	Success     bool            `json:"success"`
	Categories  map[string]bool `json:"categories"`
	CompletedAt time.Time       `json:"completed_at"`
	ReasonCodes []string        `json:"reason_codes"`
}
