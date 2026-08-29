package officialaccount

import "encoding/xml"

// Account stores the short-lived credentials captured from a public-account
// article page. The token-bearing fields are never returned by the list API.
type Account struct {
	Biz              string `json:"biz"`
	Nickname         string `json:"nickname"`
	AvatarURL        string `json:"avatar_url"`
	AuthorID         string `json:"author_id"`
	Uin              string `json:"uin"`
	Key              string `json:"key"`
	PassTicket       string `json:"pass_ticket"`
	AppmsgToken      string `json:"appmsg_token"`
	Cookie           string `json:"cookie,omitempty"`
	CookieExpiration int64  `json:"cookie_expiration,omitempty"`
	RefreshURI       string `json:"refresh_uri"`
	IsEffective      bool   `json:"is_effective"`
	CreatedAt        int64  `json:"created_at"`
	UpdateTime       int64  `json:"update_time"`
	Error            string `json:"error,omitempty"`
}

func (a *Account) MergeFrom(source Account) {
	if source.Nickname != "" {
		a.Nickname = source.Nickname
	}
	if source.AvatarURL != "" {
		a.AvatarURL = source.AvatarURL
	}
	if source.AuthorID != "" {
		a.AuthorID = source.AuthorID
	}
	if source.Uin != "" {
		a.Uin = source.Uin
	}
	if source.Key != "" {
		a.Key = source.Key
	}
	if source.PassTicket != "" {
		a.PassTicket = source.PassTicket
	}
	if source.AppmsgToken != "" {
		a.AppmsgToken = source.AppmsgToken
	}
	if source.Cookie != "" {
		a.Cookie = source.Cookie
	}
	if source.CookieExpiration != 0 {
		a.CookieExpiration = source.CookieExpiration
	}
	if source.RefreshURI != "" {
		a.RefreshURI = source.RefreshURI
	}
	if source.Error != "" {
		a.Error = source.Error
	}
}

type AccountLink struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type AccountSummary struct {
	Biz           string        `json:"biz"`
	Nickname      string        `json:"nickname"`
	AvatarURL     string        `json:"avatar_url"`
	IsEffective   bool          `json:"is_effective"`
	CreatedAt     int64         `json:"created_at"`
	UpdateTime    int64         `json:"update_time"`
	Error         string        `json:"error,omitempty"`
	RefreshURI    string        `json:"refresh_uri,omitempty"`
	ArticleCount  int64         `json:"article_count"`
	ArchivedCount int64         `json:"archived_count"`
	LastSyncAt    int64         `json:"last_sync_at,omitempty"`
	SyncStatus    string        `json:"sync_status,omitempty"`
	SyncError     string        `json:"sync_error,omitempty"`
	Links         []AccountLink `json:"links"`
}

type MessageListResponse struct {
	Ret            int           `json:"ret"`
	ErrMsg         string        `json:"errmsg"`
	GeneralMsgList string        `json:"general_msg_list"`
	CanMsgContinue int           `json:"can_msg_continue"`
	MsgCount       int           `json:"msg_count"`
	NextOffset     int           `json:"next_offset"`
	List           []MessageItem `json:"list,omitempty"`
	Articles       []ArticleItem `json:"articles,omitempty"`
}

type CommonMsgInfo struct {
	ID       int    `json:"id"`
	Type     int    `json:"type"`
	Datetime int    `json:"datetime"`
	FakeID   string `json:"fakeid"`
	Status   int    `json:"status"`
	Content  string `json:"content"`
}

type MessageItem struct {
	MsgExtInfo    MessageExtInfo `json:"app_msg_ext_info"`
	CommonMsgInfo CommonMsgInfo  `json:"comm_msg_info"`
}

type ArticleItem struct {
	Title                  string `json:"title"`
	Digest                 string `json:"digest"`
	Content                string `json:"content"`
	FileID                 int    `json:"fileid"`
	VideoID                string `json:"video_id,omitempty"`
	ContentURL             string `json:"content_url"`
	SourceURL              string `json:"source_url"`
	Cover                  string `json:"cover"`
	Author                 string `json:"author"`
	Subtype                int    `json:"subtype,omitempty"`
	Mid                    string `json:"mid,omitempty"`
	Idx                    int    `json:"idx,omitempty"`
	IsMulti                int    `json:"is_multi,omitempty"`
	IsOriginal             int    `json:"is_original,omitempty"`
	IsPaid                 int    `json:"is_paid,omitempty"`
	IsPaySubscribe         int    `json:"is_pay_subscribe,omitempty"`
	ItemShowType           int    `json:"item_show_type,omitempty"`
	CopyrightStat          int    `json:"copyright_stat,omitempty"`
	Duration               int    `json:"duration,omitempty"`
	AudioFileID            int    `json:"audio_fileid,omitempty"`
	PlayURL                string `json:"play_url,omitempty"`
	MaliciousTitleReasonID int    `json:"malicious_title_reason_id,omitempty"`
	MaliciousContentType   int    `json:"malicious_content_type,omitempty"`
	DelFlag                int    `json:"del_flag,omitempty"`
	PublishTime            int64  `json:"publish_time,omitempty"`
}

type MessageExtInfo struct {
	Title                  string        `json:"title"`
	Digest                 string        `json:"digest"`
	Content                string        `json:"content"`
	FileID                 int           `json:"fileid"`
	VideoID                string        `json:"video_id"`
	ContentURL             string        `json:"content_url"`
	SourceURL              string        `json:"source_url"`
	Cover                  string        `json:"cover"`
	Subtype                int           `json:"subtype"`
	IsMulti                int           `json:"is_multi"`
	IsOriginal             int           `json:"is_original"`
	IsPaid                 int           `json:"is_paid"`
	IsPaySubscribe         int           `json:"is_pay_subscribe"`
	MultiAppMsgItemList    []ArticleItem `json:"multi_app_msg_item_list"`
	Author                 string        `json:"author"`
	CopyrightStat          int           `json:"copyright_stat"`
	Duration               int           `json:"duration"`
	DelFlag                int           `json:"del_flag"`
	ItemShowType           int           `json:"item_show_type"`
	AudioFileID            int           `json:"audio_fileid"`
	PlayURL                string        `json:"play_url"`
	MaliciousTitleReasonID int           `json:"malicious_title_reason_id"`
	MaliciousContentType   int           `json:"malicious_content_type"`
}

type Article struct {
	IsMulti                int    `json:"is_multi"`
	IsOriginal             int    `json:"is_original"`
	IsPaid                 int    `json:"is_paid"`
	IsPaySubscribe         int    `json:"is_pay_subscribe"`
	ItemShowType           int    `json:"item_show_type"`
	Mid                    string `json:"mid"`
	PublishTime            int64  `json:"publish_time"`
	Title                  string `json:"title"`
	URL                    string `json:"url"`
	VideoID                string `json:"video_id,omitempty"`
	Subtype                int    `json:"subtype,omitempty"`
	CopyrightStat          int    `json:"copyright_stat,omitempty"`
	Duration               int    `json:"duration,omitempty"`
	AudioFileID            int    `json:"audio_fileid,omitempty"`
	PlayURL                string `json:"play_url,omitempty"`
	MaliciousTitleReasonID int    `json:"malicious_title_reason_id,omitempty"`
	MaliciousContentType   int    `json:"malicious_content_type,omitempty"`
}

type ArticleListResponse struct {
	Ret      int       `json:"ret"`
	ErrMsg   string    `json:"errmsg"`
	Articles []Article `json:"articles"`
	BaseResp struct {
		ExportKeyToken string `json:"exportkey_token"`
		Ret            int    `json:"ret"`
	} `json:"base_resp"`
	MaxArticleID string `json:"max_article_id"`
}

type AtomAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

type AtomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type AtomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",cdata"`
}

type MediaThumbnail struct {
	XMLName    xml.Name `xml:"media:thumbnail"`
	XMLNSMedia string   `xml:"xmlns:media,attr"`
	URL        string   `xml:"url,attr"`
	Width      int      `xml:"width,attr,omitempty"`
	Height     int      `xml:"height,attr,omitempty"`
}

type AtomEntry struct {
	ID             string          `xml:"id"`
	Title          string          `xml:"title"`
	Updated        string          `xml:"updated"`
	Published      string          `xml:"published"`
	Author         AtomAuthor      `xml:"author"`
	Link           []AtomLink      `xml:"link"`
	Content        AtomContent     `xml:"content"`
	Summary        AtomContent     `xml:"summary"`
	MediaThumbnail *MediaThumbnail `xml:"media:thumbnail,omitempty"`
}

type AtomCategory struct {
	Term string `xml:"term,attr"`
}

type AtomFeed struct {
	XMLName    xml.Name       `xml:"feed"`
	XMLNS      string         `xml:"xmlns,attr"`
	XMLNSMedia string         `xml:"xmlns:media,attr"`
	Title      string         `xml:"title"`
	ID         string         `xml:"id"`
	Updated    string         `xml:"updated"`
	Generator  string         `xml:"generator"`
	Icon       string         `xml:"icon,omitempty"`
	Category   []AtomCategory `xml:"category"`
	Link       []AtomLink     `xml:"link"`
	Author     AtomAuthor     `xml:"author"`
	Entry      []AtomEntry    `xml:"entry"`
}
