package services

import "strings"

// BuildSharedFeedProfileCompatResponse converts parsed share-link data into the
// same object shape used by existing feed profile consumers.
func BuildSharedFeedProfileCompatResponse(resp *SphFeedResponse) map[string]interface{} {
	if resp == nil {
		return map[string]interface{}{
			"errCode": 0,
			"errMsg":  "ok",
			"data": map[string]interface{}{
				"object": map[string]interface{}{},
			},
		}
	}

	videoURL := strings.TrimSpace(resp.Data.FeedInfo.OriginVideoURL)
	if videoURL == "" {
		videoURL = strings.TrimSpace(resp.Data.FeedInfo.VideoURL)
	}
	if videoURL == "" {
		videoURL = strings.TrimSpace(resp.Data.FeedInfo.H264VideoInfo.VideoURL)
	}
	if videoURL == "" {
		videoURL = strings.TrimSpace(resp.Data.FeedInfo.H265VideoInfo.VideoURL)
	}

	coverURL := strings.TrimSpace(resp.Data.FeedInfo.CoverURL)
	mediaType := resp.Data.FeedInfo.MediaType
	if mediaType == 0 {
		mediaType = 4
	}

	objectID := strings.TrimSpace(resp.Data.SceneInfo.DynamicExportID)
	if objectID == "" {
		objectID = "shared_feed"
	}

	contact := map[string]interface{}{
		"username":    "",
		"nickname":    resp.Data.AuthorInfo.Nickname,
		"headUrl":     resp.Data.AuthorInfo.HeadImgURL,
		"headImgUrl":  resp.Data.AuthorInfo.HeadImgURL,
		"authIconUrl": resp.Data.AuthorInfo.AuthIconURL,
		"signature":   "",
	}

	media := map[string]interface{}{
		"url":          videoURL,
		"urlToken":     "",
		"decodeKey":    "",
		"coverUrl":     coverURL,
		"thumbUrl":     coverURL,
		"fullThumbUrl": coverURL,
		"spec":         []interface{}{},
	}

	object := map[string]interface{}{
		"id":            objectID,
		"objectNonceId": "",
		"createtime":    resp.Data.FeedInfo.CreateTime,
		"username":      "",
		"nickname":      resp.Data.AuthorInfo.Nickname,
		"headUrl":       resp.Data.AuthorInfo.HeadImgURL,
		"signature":     "",
		"readCount":     0,
		"likeCount":     resp.Data.FeedInfo.LikeCountFmt,
		"favCount":      resp.Data.FeedInfo.FavCountFmt,
		"forwardCount":  resp.Data.FeedInfo.ForwardCountFmt,
		"commentCount":  resp.Data.FeedInfo.CommentCountFmt,
		"ipRegion":      "",
		"contact":       contact,
		"objectDesc": map[string]interface{}{
			"description": resp.Data.FeedInfo.Description,
			"mediaType":   mediaType,
			"media":       []interface{}{media},
		},
	}

	return map[string]interface{}{
		"errCode": 0,
		"errMsg":  "ok",
		"data": map[string]interface{}{
			"object": object,
			"sceneInfo": map[string]interface{}{
				"dynamicExportId": objectID,
			},
			"feedInfo": map[string]interface{}{
				"videoUrl":        resp.Data.FeedInfo.VideoURL,
				"originVideoUrl":  resp.Data.FeedInfo.OriginVideoURL,
				"description":     resp.Data.FeedInfo.Description,
				"mediaType":       resp.Data.FeedInfo.MediaType,
				"coverUrl":        resp.Data.FeedInfo.CoverURL,
				"createtime":      resp.Data.FeedInfo.CreateTime,
				"favCountFmt":     resp.Data.FeedInfo.FavCountFmt,
				"likeCountFmt":    resp.Data.FeedInfo.LikeCountFmt,
				"forwardCountFmt": resp.Data.FeedInfo.ForwardCountFmt,
				"commentCountFmt": resp.Data.FeedInfo.CommentCountFmt,
			},
			"authorInfo": map[string]interface{}{
				"nickname":    resp.Data.AuthorInfo.Nickname,
				"headImgUrl":  resp.Data.AuthorInfo.HeadImgURL,
				"authIconUrl": resp.Data.AuthorInfo.AuthIconURL,
			},
		},
	}
}
