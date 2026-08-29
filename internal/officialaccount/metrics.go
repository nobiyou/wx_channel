package officialaccount

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

type metricFieldDefinition struct {
	Name    string
	Aliases []string
	Labels  []string
}

type metricEvidence struct {
	Value         int64  `json:"value"`
	Source        string `json:"source"`
	Raw           string `json:"raw,omitempty"`
	AliasPriority int    `json:"-"`
}

var metricFieldDefinitions = []metricFieldDefinition{
	{
		Name: "view_count",
		Aliases: []string{
			"read_num3", "readNum3", "read_num2", "readNum2", "read_num", "readNum", "reading_count", "readingCount",
			"view_count", "viewCount", "views", "read_count", "readCount",
		},
		Labels: []string{"阅读量", "阅读", "浏览量", "浏览", "read"},
	},
	{
		Name: "like_count",
		Aliases: []string{
			"like_num", "likeNum", "like_count", "likeCount", "old_like_num", "oldLikeNum", "old_like_count",
			"praise_count", "praiseCount", "digg_count", "diggCount",
		},
		Labels: []string{"点赞数", "点赞", "喜欢", "like", "praise"},
	},
	{
		Name: "comment_count",
		Aliases: []string{
			"elected_comment_total_cnt", "electedCommentTotalCnt", "comment_total_cnt", "commentTotalCnt",
			"preload_comment_total_cnt", "preloadCommentTotalCnt", "appmsg_comment_total_cnt", "appmsgCommentTotalCnt",
			"comments_count", "commentsCount", "comment_count", "commentCount", "comment_num", "commentNum", "cmt_count", "cmtCount",
		},
		Labels: []string{"评论数", "评论", "留言", "comment"},
	},
	{
		Name: "share_count",
		Aliases: []string{
			"share_count", "shareCount", "share_num", "shareNum", "forward_count", "forwardCount",
			"repost_count", "repostCount", "repost_num", "repostNum", "forward_num", "forwardNum",
		},
		Labels: []string{"转发数", "转发", "分享数", "分享", "forward", "share", "repost"},
	},
	{
		Name: "collect_count",
		Aliases: []string{
			"collect_count", "collectCount", "collect_num", "collectNum", "favorite_count", "favoriteCount",
			"favorite_num", "favoriteNum", "fav_count", "favCount", "fav_num", "favNum", "收藏数", "收藏",
		},
		Labels: []string{"收藏数", "收藏", "favorite", "collect"},
	},
	{
		Name: "reward_count",
		Aliases: []string{
			"reward_count", "rewardCount", "reward_num", "rewardNum", "rewards_count", "rewardsCount",
			"appmsg_reward_count", "appmsgRewardCount", "tip_count", "tipCount",
		},
		Labels: []string{"赞赏数", "赞赏", "打赏数", "打赏", "reward", "tip"},
	},
}

var metricNumberPattern = regexp.MustCompile(`[-+]?[0-9][0-9, ]*(?:\.[0-9]+)?[ \t\r\n]*(?:万|千|亿|w|k|m)?`)

// ExtractArticleMetrics reads only public counter values from an article page.
// It accepts both the common inline JavaScript variables and text/attributes
// emitted by different WeChat article layouts. Unknown counters are omitted.
// The returned metadata is intentionally small and contains no HTML or URL.
func ExtractArticleMetrics(pageHTML string) (ArticleMetricPayload, string) {
	var payload ArticleMetricPayload
	if strings.TrimSpace(pageHTML) == "" {
		return payload, ""
	}
	if len(pageHTML) > maxArticleBody {
		pageHTML = pageHTML[:maxArticleBody]
	}

	evidence := make(map[string]metricEvidence, len(metricFieldDefinitions))
	if document, err := html.Parse(strings.NewReader(pageHTML)); err == nil {
		collectMetricDOMEvidence(document, evidence)
	}
	collectMetricScriptEvidence(pageHTML, evidence)
	collectMetricJSONEvidence(pageHTML, evidence)

	for _, definition := range metricFieldDefinitions {
		item, ok := evidence[definition.Name]
		if !ok {
			continue
		}
		setMetricPayloadValue(&payload, definition.Name, item.Value)
	}
	if len(evidence) == 0 {
		return payload, ""
	}
	keys := make([]string, 0, len(evidence))
	for key := range evidence {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]metricEvidence, len(evidence))
	for _, key := range keys {
		ordered[key] = evidence[key]
	}
	data, err := json.Marshal(ordered)
	if err != nil {
		return payload, ""
	}
	return payload, string(data)
}

const (
	maxMetricJSONDepth = 12
	maxMetricJSONNodes = 5000
	maxMetricJSONText  = 2 * 1024 * 1024
)

// ExtractArticleMetricsJSON extracts counters from a raw WeChat JSON response.
// The response is intentionally treated as untrusted input: it may contain
// nested objects, arrays, or a JSON document encoded inside a string.
func ExtractArticleMetricsJSON(raw string) (ArticleMetricPayload, string) {
	evidence := extractArticleMetricJSONEvidence(raw)
	return metricPayloadAndEvidence(evidence)
}

func extractArticleMetricJSONEvidence(raw string) map[string]metricEvidence {
	evidence := make(map[string]metricEvidence, len(metricFieldDefinitions))
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxMetricJSONText {
		return evidence
	}
	var value interface{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return evidence
	}
	nodes := 0
	walkMetricJSON(value, 0, &nodes, evidence)
	return evidence
}

func walkMetricJSON(value interface{}, depth int, nodes *int, evidence map[string]metricEvidence) {
	if value == nil || nodes == nil || evidence == nil || depth > maxMetricJSONDepth || *nodes >= maxMetricJSONNodes {
		return
	}
	*nodes = *nodes + 1
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || len(text) > maxMetricJSONText || (text[0] != '{' && text[0] != '[' && text[0] != '"') {
			return
		}
		var nested interface{}
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if err := decoder.Decode(&nested); err == nil {
			walkMetricJSON(nested, depth+1, nodes, evidence)
		}
	case []interface{}:
		for i, child := range typed {
			if i >= 256 || *nodes >= maxMetricJSONNodes {
				break
			}
			walkMetricJSON(child, depth+1, nodes, evidence)
		}
	case map[string]interface{}:
		for key, child := range typed {
			if name, ok := metricNameForJSONKey(key); ok {
				if value, rawValue, parsed := metricNumberFromJSONValue(child); parsed {
					putMetricEvidence(evidence, name, metricEvidence{
						Value: value, Source: "network_json", Raw: rawValue,
						AliasPriority: metricAliasPriority(name, key),
					})
				}
			}
			if *nodes >= maxMetricJSONNodes {
				break
			}
			walkMetricJSON(child, depth+1, nodes, evidence)
		}
	}
}

func metricNameForJSONKey(key string) (string, bool) {
	for _, definition := range metricFieldDefinitions {
		for _, alias := range definition.Aliases {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(alias)) {
				return definition.Name, true
			}
		}
	}
	return "", false
}

func metricNumberFromJSONValue(value interface{}) (int64, string, bool) {
	var raw string
	switch typed := value.(type) {
	case json.Number:
		raw = typed.String()
	case string:
		raw = typed
	default:
		return 0, "", false
	}
	parsed, normalized, ok := parseMetricNumber(raw)
	if !ok {
		return 0, "", false
	}
	if len(normalized) > 128 {
		normalized = normalized[:128]
	}
	return parsed, normalized, true
}

func metricPayloadAndEvidence(evidence map[string]metricEvidence) (ArticleMetricPayload, string) {
	var payload ArticleMetricPayload
	if len(evidence) == 0 {
		return payload, ""
	}
	keys := make([]string, 0, len(evidence))
	for key := range evidence {
		keys = append(keys, key)
		setMetricPayloadValue(&payload, key, evidence[key].Value)
	}
	sort.Strings(keys)
	ordered := make(map[string]metricEvidence, len(evidence))
	for _, key := range keys {
		ordered[key] = evidence[key]
	}
	data, err := json.Marshal(ordered)
	if err != nil {
		return payload, ""
	}
	return payload, string(data)
}

func collectMetricJSONEvidence(pageHTML string, evidence map[string]metricEvidence) {
	if strings.TrimSpace(pageHTML) == "" {
		return
	}
	scriptPattern := regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script\s*>`)
	for _, match := range scriptPattern.FindAllStringSubmatch(pageHTML, 200) {
		if len(match) > 1 {
			collectMetricJSONCandidates(match[1], evidence)
		}
	}
	trimmed := strings.TrimSpace(pageHTML)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		collectMetricJSONCandidates(trimmed, evidence)
	}
}

func collectMetricJSONCandidates(source string, evidence map[string]metricEvidence) {
	source = strings.TrimSpace(source)
	if source == "" || len(source) > maxMetricJSONText {
		return
	}
	if payloadEvidence := extractArticleMetricJSONEvidence(source); len(payloadEvidence) > 0 {
		for name, item := range payloadEvidence {
			putMetricEvidence(evidence, name, item)
		}
	}
	for start := 0; start < len(source); start++ {
		if source[start] != '{' && source[start] != '[' {
			continue
		}
		end, ok := balancedJSONEnd(source, start)
		if !ok {
			continue
		}
		candidate := source[start:end]
		if payloadEvidence := extractArticleMetricJSONEvidence(candidate); len(payloadEvidence) > 0 {
			for name, item := range payloadEvidence {
				putMetricEvidence(evidence, name, item)
			}
		}
		start = end - 1
	}
}

func balancedJSONEnd(source string, start int) (int, bool) {
	if start < 0 || start >= len(source) || (source[start] != '{' && source[start] != '[') {
		return 0, false
	}
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for index := start; index < len(source); index++ {
		char := source[index]
		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, char)
		case '}', ']':
			if len(stack) == 0 || (char == '}' && stack[len(stack)-1] != '{') || (char == ']' && stack[len(stack)-1] != '[') {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return index + 1, true
			}
		}
	}
	return 0, false
}

func collectMetricDOMEvidence(node *html.Node, evidence map[string]metricEvidence) {
	if node == nil {
		return
	}
	if isNonRenderedMetricElement(node) {
		return
	}
	if node.Type == html.ElementNode {
		identityParts := make([]string, 0, 4)
		for _, attribute := range node.Attr {
			name := strings.ToLower(strings.TrimSpace(attribute.Key))
			if name == "id" || name == "class" || strings.HasPrefix(name, "data-") {
				identityParts = append(identityParts, strings.ToLower(attribute.Val))
			}
		}
		identity := strings.Join(identityParts, " ")
		text := strings.TrimSpace(nodeText(node))
		if identity != "" || text != "" {
			for _, definition := range metricFieldDefinitions {
				if current, exists := evidence[definition.Name]; exists && current.Source == "dom" {
					continue
				}
				value, raw, ok := metricValueFromDOM(identity, text, definition)
				if ok {
					putMetricEvidence(evidence, definition.Name, metricEvidence{
						Value: value, Source: "dom", Raw: raw,
						AliasPriority: metricIdentityAliasPriority(definition.Name, identity),
					})
				}
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectMetricDOMEvidence(child, evidence)
	}
}

func metricValueFromDOM(identity, text string, definition metricFieldDefinition) (int64, string, bool) {
	if text != "" {
		lowerText := strings.ToLower(text)
		for _, label := range definition.Labels {
			index := strings.Index(lowerText, strings.ToLower(label))
			if index >= 0 {
				if value, raw, ok := parseMetricNumber(text[index+len(label):]); ok {
					return value, raw, true
				}
			}
		}
	}
	if identity == "" {
		return 0, "", false
	}
	if metricIdentityAliasPriority(definition.Name, identity) > 0 {
		if value, raw, ok := parseMetricNumber(text); ok {
			return value, raw, true
		}
	}
	return 0, "", false
}

func collectMetricScriptEvidence(pageHTML string, evidence map[string]metricEvidence) {
	for _, definition := range metricFieldDefinitions {
		if current, exists := evidence[definition.Name]; exists && current.Source == "dom" {
			continue
		}
		for _, alias := range definition.Aliases {
			pattern := regexp.MustCompile(`(?i)(?:^|[^[:alnum:]_-])["']?` + regexp.QuoteMeta(alias) + `["']?\s*(?::|=)\s*["']?(` + metricNumberPattern.String() + `)`)
			match := pattern.FindStringSubmatch(pageHTML)
			if len(match) < 2 {
				continue
			}
			if value, raw, ok := parseMetricNumber(match[1]); ok {
				putMetricEvidence(evidence, definition.Name, metricEvidence{
					Value: value, Source: "script", Raw: raw,
					AliasPriority: metricAliasPriority(definition.Name, alias),
				})
				break
			}
		}
	}
}

func putMetricEvidence(evidence map[string]metricEvidence, name string, item metricEvidence) {
	if evidence == nil {
		return
	}
	current, exists := evidence[name]
	if !exists || metricEvidenceBetter(item, current) {
		evidence[name] = item
	}
}

func metricEvidenceBetter(candidate, current metricEvidence) bool {
	candidateSourcePriority := metricEvidencePriority(candidate.Source)
	currentSourcePriority := metricEvidencePriority(current.Source)
	if candidateSourcePriority != currentSourcePriority {
		return candidateSourcePriority > currentSourcePriority
	}
	return candidate.AliasPriority > current.AliasPriority
}

func metricAliasPriority(name, key string) int {
	key = strings.TrimSpace(key)
	for _, definition := range metricFieldDefinitions {
		if definition.Name != name {
			continue
		}
		for index, alias := range definition.Aliases {
			if strings.EqualFold(key, alias) {
				return len(definition.Aliases) - index
			}
		}
	}
	return 0
}

func metricIdentityAliasPriority(name, identity string) int {
	identity = strings.ToLower(strings.TrimSpace(identity))
	if identity == "" {
		return 0
	}
	for _, definition := range metricFieldDefinitions {
		if definition.Name != name {
			continue
		}
		exactPriority := 0
		embeddedPriority := 0
		for index, alias := range definition.Aliases {
			priority := len(definition.Aliases) - index
			if metricIdentityTokenMatches(identity, alias) {
				if priority > exactPriority {
					exactPriority = priority
				}
				continue
			}
			if containsMetricToken(identity, alias) && priority > embeddedPriority {
				embeddedPriority = priority
			}
		}
		if exactPriority > 0 {
			return exactPriority
		}
		return embeddedPriority
	}
	return 0
}

func metricIdentityTokenMatches(identity, alias string) bool {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return false
	}
	for _, token := range strings.Fields(identity) {
		if token == alias {
			return true
		}
	}
	return false
}

func metricEvidencePriority(source string) int {
	switch source {
	case "network_json":
		return 3
	case "script":
		return 2
	case "dom":
		return 1
	default:
		return 0
	}
}

func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if isNonRenderedMetricElement(node) {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if value := nodeText(child); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

func isNonRenderedMetricElement(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "script", "style", "noscript", "template":
		return true
	default:
		return false
	}
}

func containsMetricToken(value, token string) bool {
	value = strings.ToLower(value)
	token = strings.ToLower(token)
	if strings.ContainsAny(token, "_-") {
		return strings.Contains(value, token)
	}
	start := 0
	for {
		index := strings.Index(value[start:], token)
		if index < 0 {
			return false
		}
		index += start
		beforeOK := index == 0 || !isMetricIdentifierRune(rune(value[index-1]))
		end := index + len(token)
		afterOK := end >= len(value) || !isMetricIdentifierRune(rune(value[end]))
		if beforeOK && afterOK {
			return true
		}
		start = end
		if start >= len(value) {
			return false
		}
	}
}

func isMetricIdentifierRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' || value == '-'
}

func parseMetricNumber(raw string) (int64, string, bool) {
	match := metricNumberPattern.FindString(raw)
	if match == "" {
		return 0, "", false
	}
	normalized := strings.TrimSpace(strings.ReplaceAll(match, ",", ""))
	normalized = strings.Join(strings.Fields(normalized), "")
	multiplier := float64(1)
	if len(normalized) > 0 {
		lower := strings.ToLower(normalized)
		switch {
		case strings.HasSuffix(lower, "万"):
			multiplier = 10000
			normalized = strings.TrimSuffix(normalized, "万")
		case strings.HasSuffix(lower, "w"):
			multiplier = 10000
			normalized = strings.TrimSuffix(normalized, "w")
		case strings.HasSuffix(lower, "千"):
			multiplier = 1000
			normalized = strings.TrimSuffix(normalized, "千")
		case strings.HasSuffix(lower, "k"):
			multiplier = 1000
			normalized = strings.TrimSuffix(normalized, "k")
		case strings.HasSuffix(lower, "亿"):
			multiplier = 100000000
			normalized = strings.TrimSuffix(normalized, "亿")
		case strings.HasSuffix(lower, "m"):
			multiplier = 1000000
			normalized = strings.TrimSuffix(normalized, "m")
		}
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil || value < 0 || value > float64(math.MaxInt64)/multiplier {
		return 0, "", false
	}
	value *= multiplier
	return int64(math.Round(value)), strings.TrimSpace(match), true
}

func setMetricPayloadValue(payload *ArticleMetricPayload, name string, value int64) {
	if payload == nil {
		return
	}
	copy := value
	switch name {
	case "view_count":
		payload.ViewCount = &copy
	case "like_count":
		payload.LikeCount = &copy
	case "comment_count":
		payload.CommentCount = &copy
	case "share_count":
		payload.ShareCount = &copy
	case "collect_count":
		payload.CollectCount = &copy
	case "reward_count":
		payload.RewardCount = &copy
	}
}

func metricPayloadEmpty(payload ArticleMetricPayload) bool {
	return payload.ViewCount == nil && payload.LikeCount == nil && payload.CommentCount == nil &&
		payload.ShareCount == nil && payload.CollectCount == nil && payload.RewardCount == nil
}

func mergeMetricPayload(preferred, fallback ArticleMetricPayload) ArticleMetricPayload {
	merged := preferred
	if merged.ViewCount == nil {
		merged.ViewCount = fallback.ViewCount
	}
	if merged.LikeCount == nil {
		merged.LikeCount = fallback.LikeCount
	}
	if merged.CommentCount == nil {
		merged.CommentCount = fallback.CommentCount
	}
	if merged.ShareCount == nil {
		merged.ShareCount = fallback.ShareCount
	}
	if merged.CollectCount == nil {
		merged.CollectCount = fallback.CollectCount
	}
	if merged.RewardCount == nil {
		merged.RewardCount = fallback.RewardCount
	}
	return merged
}

// mergeMetricPayloadWithSnapshot fills only fields omitted by the current
// observation. An explicit zero remains authoritative and is never replaced.
func mergeMetricPayloadWithSnapshot(current ArticleMetricPayload, previous *ArticleMetricSnapshot) ArticleMetricPayload {
	if previous == nil || metricPayloadEmpty(current) {
		return current
	}
	return mergeMetricPayload(current, ArticleMetricPayload{
		ViewCount:    previous.ViewCount,
		LikeCount:    previous.LikeCount,
		CommentCount: previous.CommentCount,
		ShareCount:   previous.ShareCount,
		CollectCount: previous.CollectCount,
		RewardCount:  previous.RewardCount,
	})
}
