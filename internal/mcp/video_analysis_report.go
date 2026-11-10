package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) getVideoAnalysisStatus(ctx context.Context, _ *mcp.CallToolRequest, input videoAnalysisStatusInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	job, err := s.loadAnalysisJob(input.AnalysisID)
	if err != nil {
		return nil, nil, err
	}
	s.analysisMu.Lock()
	_, active := s.analysisJobs[job.ID]
	s.analysisMu.Unlock()
	freshness := "stored"
	if active {
		freshness = "live"
	}
	return nil, envelope("local_video_analysis", freshness, analysisJobSummary(job.Manifest), job.Manifest.Warnings), nil
}

func (s *Server) writeAnalysisReports(manifest *videoAnalysisManifest) error {
	if manifest == nil {
		return analysisStorageError()
	}
	manifest.AnalysisProvider = analysisProvider(*manifest)
	manifest.ReviewStatus = analysisReviewStatus(*manifest)
	ensureVideoAnalysisDetails(manifest)
	root, err := s.analysisRoot(manifest.AnalysisID, false)
	if err != nil {
		return err
	}
	defer root.Close()
	report := buildVideoAnalysisReport(*manifest)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil || len(data) > maxAnalysisManifestBytes {
		return analysisStorageError()
	}
	if err := writeAnalysisFile(root, "report.json", data); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>视频拍摄还原报告</title><style>body{font:14px system-ui,-apple-system,sans-serif;max-width:1240px;margin:2rem auto;padding:0 1rem;line-height:1.6;color:#202124}h1{margin-bottom:.35rem}h2{margin-top:2.2rem;border-bottom:2px solid #1a73e8;padding-bottom:.35rem}h3{margin:.2rem 0}.muted{color:#666}.card{border:1px solid #d9e0e7;border-radius:10px;padding:14px;margin:12px 0;background:#fff}.notice{border-left:4px solid #f9ab00;background:#fff8e1;padding:10px 12px;margin:10px 0}.ok{border-left-color:#188038;background:#e6f4ea}.table-wrap{overflow:auto;margin:10px 0}table{border-collapse:collapse;width:100%;min-width:680px}td,th{border:1px solid #d9e0e7;padding:.55rem .65rem;text-align:left;vertical-align:top}th{background:#f5f7fa;font-weight:600}.details-table{min-width:0}.details-table th{width:180px}.gallery{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:10px;margin:10px 0}.gallery figure{margin:0;border:1px solid #e1e5ea;border-radius:8px;padding:6px;background:#fafafa}.gallery img{display:block;width:100%;max-height:320px;object-fit:contain;background:#eee}.gallery figcaption{font-size:12px;color:#555;margin-top:4px}.shot-card{border:1px solid #d9e0e7;border-radius:10px;padding:14px;margin:14px 0}.shot-meta{color:#555}.analysis-table{min-width:0}.analysis-table th{width:150px}pre,dd{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;margin:0}.script-table{font-size:13px}.script-table td,.script-table th{min-width:110px}.small{font-size:12px;color:#666}.pill{display:inline-block;border-radius:999px;background:#eef3fe;padding:2px 8px;margin-right:4px}</style></head><body>")
	providerLabel := map[string]string{"codex": "Codex 当前会话模型", "local": "本地证据", "gemini": "Gemini API", "openai_compatible": "第三方 API", "external": "外部 API"}[manifest.AnalysisProvider]
	if providerLabel == "" {
		providerLabel = manifest.AnalysisProvider
	}
	b.WriteString("<style>td{white-space:pre-wrap;overflow-wrap:anywhere}.gallery figcaption{line-height:1.65}.analysis-table th{width:130px}@media(max-width:600px){body{margin:1rem auto;padding:0 .8rem}.details-table th,.analysis-table th{width:90px}h1{font-size:24px}h2{font-size:20px}}</style>")
	reviewLabel := map[string]string{"preparing": "正在准备关键帧", "awaiting_codex_review": "待 Codex 分析", "partial": "部分镜头已分析", "completed": "分析完成", "not_requested": "仅提取本地证据", "unavailable": "分析准备失败"}[manifest.ReviewStatus]
	b.WriteString("<h1>视频拍摄还原报告</h1><p class=\"muted\">分析 ID: " + html.EscapeString(manifest.AnalysisID) + "；分析来源: " + html.EscapeString(providerLabel) + "；进度: " + html.EscapeString(reviewLabel) + "</p>")
	b.WriteString("<p>报告将本地可验证证据、Codex/模型复核和制作建议分开呈现。镜头边界是检测候选；关键帧只证明截图时刻，连续动作、声音和具体设备需要结合时间线复核。</p>")

	b.WriteString("<h2 id=\"video-details\">一、视频详情</h2>")
	writeVideoDetailsTable(&b, manifest.VideoDetails)

	b.WriteString("<h2 id=\"visual-analysis\">二、视频画面截图及分析</h2>")
	b.WriteString("<p class=\"muted\">每个镜头展示全部关键帧，时间戳位于镜头区间内。图像是视觉证据，字段中的推断会标明来源。</p>")
	if len(manifest.Shots) == 0 {
		b.WriteString("<div class=\"notice\">没有生成镜头关键帧。</div>")
	}
	for _, shot := range manifest.Shots {
		b.WriteString("<article class=\"shot-card\"><h3>镜头 " + fmt.Sprintf("%d", shot.Index) + "</h3>")
		b.WriteString(fmt.Sprintf("<p class=\"shot-meta\">%.3fs–%.3fs；时长 %.3fs；转场：%s</p>", shot.Start, shot.End, shot.Duration, html.EscapeString(shot.TransitionIn)))
		b.WriteString("<div class=\"gallery\">")
		for frameIndex, frame := range shot.Keyframes {
			imagePath := filepath.ToSlash(filepath.Join("shots", filepath.Base(frame.RelativePath)))
			b.WriteString("<figure><a href=\"" + html.EscapeString(imagePath) + "\"><img src=\"" + html.EscapeString(imagePath) + "\" alt=\"镜头 " + fmt.Sprintf("%d", shot.Index) + " 关键帧\"></a><figcaption>关键帧 " + fmt.Sprintf("%d · %.3fs · %dx%d", frameIndex+1, frame.Timestamp, frame.Width, frame.Height))
			for _, observation := range analysisFieldSlice[analysisFrameObservation](preferredShotValues(shot)["frame_analyses"]) {
				if math.Abs(observation.Timestamp-frame.Timestamp) < 0.001 {
					b.WriteString("<p>" + html.EscapeString(observation.Description) + "</p>")
				}
			}
			b.WriteString("</figcaption></figure>")
		}
		b.WriteString("</div>")
		writeShotAnalysisTable(&b, shot)
		b.WriteString("</article>")
	}

	b.WriteString("<h2 id=\"speech-subtitles\">三、视频语音或字幕还原</h2>")
	writeSpeechSubtitleSection(&b, manifest)

	b.WriteString("<h2 id=\"shot-breakdown\">四、画面拆解</h2>")
	writeShotBreakdownTable(&b, manifest.Shots)

	b.WriteString("<h2 id=\"storyboard\">五、视频还原及分镜脚本</h2>")
	b.WriteString("<p class=\"muted\">分镜脚本用于复刻和二次制作。没有证据支持的内容保留为“待确认”，不会被当作原视频事实。</p>")
	writeStoryboardTable(&b, manifest)

	b.WriteString("</body></html>")
	return writeAnalysisFile(root, "report.html", []byte(b.String()))
}

func ensureVideoAnalysisDetails(manifest *videoAnalysisManifest) {
	if manifest == nil {
		return
	}
	details := &manifest.VideoDetails
	if details.RelativePath == "" {
		details.RelativePath = manifest.RelativePath
	}
	if details.FileName == "" {
		details.FileName = filepath.Base(manifest.RelativePath)
	}
	if details.FileSizeBytes == 0 {
		details.FileSizeBytes = manifest.SourceSize
	}
	if details.RecordID == "" {
		details.RecordID = manifest.RecordID
	}
	if details.Source == "" {
		details.Source = manifest.RecordSource
	}
	if details.AnalysisStatus == "" {
		details.AnalysisStatus = manifest.Status
	}
	if details.AnalysisStartedAt == "" {
		details.AnalysisStartedAt = manifest.CreatedAt
	}
	if details.AnalysisCompletedAt == "" {
		details.AnalysisCompletedAt = manifest.CompletedAt
	}
	details.AnalysisProvider = analysisProvider(*manifest)
	details.Model = manifest.Model
	if details.DurationSeconds == 0 {
		details.DurationSeconds = manifest.DurationSeconds
	}
	if details.Width == 0 {
		details.Width = manifest.Width
	}
	if details.Height == 0 {
		details.Height = manifest.Height
	}
	if details.Resolution == "" {
		details.Resolution = manifest.Resolution
	}
	if details.AudioStatus == "" {
		details.AudioStatus = audioStatus(manifest.Audio)
	}
	if details.AudioStreamPresent == nil {
		if present, ok := manifest.Audio["stream_present"].(bool); ok && (present || audioStatus(manifest.Audio) == "no_audio") {
			details.AudioStreamPresent = &present
		}
	}
	for _, value := range []*string{&details.Title, &details.Author, &details.VideoID, &details.RecordID, &details.Format, &details.Resolution, &details.FileName, &details.Model} {
		*value = safeAnalysisDetail(*value)
	}
	if !isSafeRelativePath(details.RelativePath) || localAbsolutePath.MatchString(details.RelativePath) {
		details.RelativePath = "[redacted_local_path]"
	}
}

func writeVideoDetailsTable(b *strings.Builder, details videoAnalysisDetails) {
	rows := [][2]string{
		{"标题", details.Title},
		{"作者", details.Author},
		{"视频 ID", details.VideoID},
		{"来源", details.Source},
		{"记录 ID", details.RecordID},
		{"文件路径（相对下载目录）", details.RelativePath},
		{"文件名", details.FileName},
		{"文件大小", formatAnalysisBytes(details.FileSizeBytes)},
		{"格式 / MIME", firstNonEmpty(details.Format, "待确认") + " / " + firstNonEmpty(details.MIMEType, "待确认")},
		{"时长", formatAnalysisSeconds(details.DurationSeconds)},
		{"分辨率", firstNonEmpty(details.Resolution, "待确认")},
		{"宽 × 高", formatAnalysisDimensions(details.Width, details.Height)},
		{"音频流", analysisAudioPresence(details.AudioStreamPresent, details.AudioStatus)},
		{"播放量", formatAnalysisMetric(details.PlayCount, details.MetricsAvailability["play_count"])},
		{"点赞量", formatAnalysisMetric(details.LikeCount, details.MetricsAvailability["like_count"])},
		{"评论量", formatAnalysisMetric(details.CommentCount, details.MetricsAvailability["comment_count"])},
		{"收藏量", formatAnalysisMetric(details.FavoriteCount, details.MetricsAvailability["favorite_count"])},
		{"转发量", formatAnalysisMetric(details.ForwardCount, details.MetricsAvailability["forward_count"])},
		{"互动数据来源", firstNonEmpty(details.MetricsSource, "待确认")},
		{"互动数据采集时间", analysisOptionalString(details.MetricsObservedAt)},
		{"互动数据状态", analysisMetricsStatusLabel(details.MetricsStatus)},
		{"分析状态", firstNonEmpty(details.AnalysisStatus, "待确认")},
		{"分析时间", firstNonEmpty(details.AnalysisCompletedAt, details.AnalysisStartedAt, "待确认")},
		{"分析来源 / 模型", firstNonEmpty(details.AnalysisProvider, "待确认") + " / " + analysisModelLabel(details)},
	}
	b.WriteString("<div class=\"table-wrap\"><table class=\"details-table\"><tbody>")
	for _, row := range rows {
		if strings.TrimSpace(row[1]) == "" {
			row[1] = "待确认"
		}
		b.WriteString("<tr><th>" + html.EscapeString(row[0]) + "</th><td>" + html.EscapeString(row[1]) + "</td></tr>")
	}
	b.WriteString("</tbody></table></div>")
}

func formatAnalysisMetric(value *int64, availability string) string {
	if value != nil {
		return strconv.FormatInt(*value, 10)
	}
	switch availability {
	case "not_returned":
		return "未返回"
	case "unavailable":
		return "未关联采集记录"
	default:
		return "待确认"
	}
}

func analysisOptionalString(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "待确认"
	}
	return *value
}

func analysisMetricsStatusLabel(status string) string {
	switch status {
	case "observed":
		return "已采集"
	case "partial":
		return "部分返回"
	case "not_returned":
		return "采集接口未返回"
	case "unknown":
		return "待确认（历史记录缺少来源）"
	case "unassociated":
		return "待确认（未关联采集记录）"
	default:
		return "待确认"
	}
}

func writeShotAnalysisTable(b *strings.Builder, shot analysisShot) {
	values := preferredShotValues(shot)
	rows := [][2]string{
		{"来源", firstNonEmpty(shotAnalysisSource(shot), "待 Codex/模型复核")},
		{"画面描述", reportValueText(values["description"])},
		{"主体", firstNonEmpty(reportValueText(values["subject"]), "待确认")},
		{"动作", firstNonEmpty(reportValueText(values["action"]), "待确认")},
		{"景别", firstNonEmpty(reportValueText(values["shot_size"]), "待确认")},
		{"机位", firstNonEmpty(reportValueText(values["camera_angle"]), "待确认")},
		{"构图", firstNonEmpty(reportValueText(values["composition"]), "待确认")},
		{"运镜", firstNonEmpty(reportValueText(values["camera_move"]), "待确认")},
		{"灯光", firstNonEmpty(reportValueText(values["lighting"]), "待确认")},
		{"色彩", firstNonEmpty(reportValueText(values["color"]), "待确认")},
		{"可见字幕与贴纸", firstNonEmpty(reportValueText(values["overlay_text"]), reportValueText(values["subtitle"]), "待确认")},
		{"证据与置信度", firstNonEmpty(reportValueText(values["evidence"]), reportValueText(shot.Evidence), "待确认") + "；置信度：" + firstNonEmpty(reportValueText(values["confidence"]), "待确认")},
	}
	for _, field := range [][2]string{{"label_probs", "制作方式判断"}, {"material_origin", "素材来源判断"}, {"embedded_assets", "嵌入素材"}, {"ai_edits", "AI 处理判断"}} {
		if text := reportValueText(values[field[0]]); text != "" {
			rows = append(rows, [2]string{field[1], text})
		}
	}
	b.WriteString("<div class=\"table-wrap\"><table class=\"analysis-table\"><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><th>" + html.EscapeString(row[0]) + "</th><td>" + html.EscapeString(firstNonEmpty(row[1], "待确认")) + "</td></tr>")
	}
	b.WriteString("</tbody></table></div>")
}

func writeSpeechSubtitleSection(b *strings.Builder, manifest *videoAnalysisManifest) {
	if manifest == nil {
		return
	}
	asr := manifest.LocalEnrichment.ASR
	ocr := manifest.LocalEnrichment.OCR
	writeReviewedSubtitleTimeline(b, manifest)
	b.WriteString("<details class=\"card\"><summary>语音识别原稿（待校对） · " + html.EscapeString(asr.Status) + fmt.Sprintf(" · %d 段</summary><h3>语音转写</h3>", len(asr.Segments)))
	if len(asr.Segments) > 0 {
		b.WriteString("<p class=\"muted\">本地语音识别原稿，尚未逐字校对；请与下方字幕复核结果对照。</p>")
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr><th>时间</th><th>转写内容</th></tr></thead><tbody>")
		for _, segment := range asr.Segments {
			b.WriteString(fmt.Sprintf("<tr><td>%.3fs–%.3fs</td><td>%s</td></tr>", segment.Start, segment.End, html.EscapeString(segment.Text)))
		}
		b.WriteString("</tbody></table></div>")
		if asr.Status != "completed" || asr.Truncated {
			writeMissingEvidenceNotice(b, "本地 ASR（已有部分结果）", asr.Status, firstNonEmpty(asr.MissingReason, asr.Error, "转写结果已截断"))
		}
	} else {
		writeMissingEvidenceNotice(b, "本地 ASR", asr.Status, firstNonEmpty(asr.MissingReason, asr.Error, "未返回转写结果"))
	}
	b.WriteString("</details><details class=\"card\"><summary>OCR 识别原稿（待校对） · " + html.EscapeString(ocr.Status) + fmt.Sprintf(" · %d 帧</summary><h3>字幕与画面文字</h3>", ocr.FrameCount))
	ocrTextCount := 0
	for _, frame := range ocr.Frames {
		if len(frame.Texts) == 0 {
			continue
		}
		ocrTextCount += len(frame.Texts)
	}
	if ocrTextCount > 0 {
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr><th>镜头 / 时间</th><th>识别文字</th><th>置信度</th></tr></thead><tbody>")
		for _, frame := range ocr.Frames {
			for _, text := range frame.Texts {
				stamp := fmt.Sprintf("镜头 %d / %.3fs", frame.ShotIndex, frame.Timestamp)
				prefix := analysisReference(manifest.AnalysisID, "") + "/"
				if strings.HasPrefix(frame.RelativePath, prefix) {
					path := strings.TrimPrefix(frame.RelativePath, prefix)
					if isSafeRelativePath(path) && (strings.HasPrefix(path, "ocr/") || strings.HasPrefix(path, "shots/")) {
						stamp = "<a href=\"" + html.EscapeString(path) + "\">" + stamp + "</a>"
					}
				}
				b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%.2f</td></tr>", stamp, html.EscapeString(text.Text), text.Confidence))
			}
		}
		b.WriteString("</tbody></table></div>")
		if ocr.Status != "completed" || ocr.Truncated {
			writeMissingEvidenceNotice(b, "本地 OCR（已有部分结果）", ocr.Status, firstNonEmpty(ocr.MissingReason, ocr.Error, "部分关键帧未识别或结果已截断"))
		}
	} else {
		writeMissingEvidenceNotice(b, "本地 OCR", ocr.Status, firstNonEmpty(ocr.MissingReason, ocr.Error, "未识别到字幕或画面文字"))
	}
	b.WriteString("</details>")
	if manifest.Audio != nil {
		b.WriteString("<div class=\"card\"><h3>音频证据</h3><p>状态：" + html.EscapeString(audioStatus(manifest.Audio)) + "；音频流：" + html.EscapeString(analysisAudioPresenceFromMap(manifest.Audio)) + "</p>")
		if value := reportValueText(manifest.Audio["mean_volume_db"]); value != "" {
			b.WriteString("<p>平均响度：" + html.EscapeString(value) + " dB</p>")
		}
		if value := reportValueText(manifest.Audio["silence_segments"]); value != "" && value != "[]" {
			b.WriteString("<p>静音片段：" + html.EscapeString(value) + "</p>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("<div class=\"notice\"><strong>复核边界：</strong>本地 ASR/OCR 不可用时，报告不会把缺失写成空结果。Codex 视觉复核可以补充可见字幕、对白线索和语音描述，但这些内容会标为复核线索，不等同于本地逐字转写。</div>")
	writeReviewSpeechClues(b, manifest.Shots)
	if len(manifest.LocalEnrichment.MissingReasons) > 0 {
		b.WriteString("<p class=\"small\">本地增强缺失原因：" + html.EscapeString(strings.Join(manifest.LocalEnrichment.MissingReasons, "、")) + "</p>")
	}
}

func writeReviewedSubtitleTimeline(b *strings.Builder, manifest *videoAnalysisManifest) {
	lines := []analysisTranscriptLine{}
	for _, line := range buildVideoAnalysisReport(*manifest).SpeechSubtitles.Timeline {
		if line.Source == "codex_visible_text" || line.Source == "codex_audio_review" {
			lines = append(lines, line)
		}
	}
	b.WriteString("<div class=\"card\"><h3>字幕复核结果</h3><p class=\"muted\">优先查看已复核文字。画面字幕与听音结果分别标注；截图时刻不代表字幕的精确入出点。机器原稿保留在下方对照区。</p>")
	if len(lines) > 0 {
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr><th>时间</th><th>说话人 / 类型</th><th>复核文字</th><th>证据来源</th></tr></thead><tbody>")
		for _, line := range lines {
			stamp := fmt.Sprintf("%.3fs–%.3fs", line.Start, line.End)
			if line.Start == line.End {
				stamp = fmt.Sprintf("%.3fs（截图时刻）", line.Start)
			}
			kind := map[string]string{"speech": "语音", "subtitle": "字幕", "onscreen_text": "画面文字"}[line.Kind]
			b.WriteString("<tr><td>" + html.EscapeString(stamp) + "</td><td>" + html.EscapeString(strings.TrimSpace(line.Speaker+" / "+kind)) + "</td><td>" + html.EscapeString(line.Text) + "</td><td>" + html.EscapeString(analysisTranscriptSource(line.Source)) + "</td></tr>")
		}
		b.WriteString("</tbody></table></div>")
	} else {
		b.WriteString("<p>尚无已复核字幕。请核对下方 ASR/OCR 原稿和采样截图；识别完成不表示逐字正确。</p>")
	}
	b.WriteString("</div>")
}

func writeReviewSpeechClues(b *strings.Builder, shots []analysisShot) {
	rows := make([][3]string, 0)
	for _, shot := range shots {
		values := preferredShotValues(shot)
		clue := firstNonEmpty(reportValueText(values["dialogue"]), reportValueText(values["speech"]), reportValueText(values["subtitle"]), reportValueText(values["overlay_text"]))
		if clue == "" {
			clue = firstNonEmpty(reportValueText(values["evidence"]), reportValueText(values["description"]))
		}
		if clue == "" {
			continue
		}
		rows = append(rows, [3]string{fmt.Sprintf("镜头 %d", shot.Index), fmt.Sprintf("%.3fs–%.3fs", shot.Start, shot.End), clue})
	}
	if len(rows) == 0 {
		return
	}
	b.WriteString("<div class=\"card\"><h3>Codex/模型复核线索（待与原声核对）</h3><div class=\"table-wrap\"><table><thead><tr><th>镜头</th><th>时间</th><th>线索</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>" + html.EscapeString(row[0]) + "</td><td>" + html.EscapeString(row[1]) + "</td><td>" + html.EscapeString(row[2]) + "</td></tr>")
	}
	b.WriteString("</tbody></table></div></div>")
}

func writeShotBreakdownTable(b *strings.Builder, shots []analysisShot) {
	b.WriteString("<div class=\"table-wrap\"><table><thead><tr><th>镜头 / 时间</th><th>景别</th><th>主体与动作</th><th>构图</th><th>运镜</th><th>灯光</th><th>色彩</th><th>字幕 / 证据</th><th>置信度</th></tr></thead><tbody>")
	for _, shot := range shots {
		values := preferredShotValues(shot)
		cells := []string{
			fmt.Sprintf("镜头 %d<br>%.3fs–%.3fs", shot.Index, shot.Start, shot.End),
			firstNonEmpty(reportValueText(values["shot_size"]), "待确认"),
			firstNonEmpty(reportValueText(values["subject"]), reportValueText(values["description"]), "待确认") + "\n动作：" + firstNonEmpty(reportValueText(values["action"]), "待确认"),
			firstNonEmpty(reportValueText(values["composition"]), "待确认") + "\n机位：" + firstNonEmpty(reportValueText(values["camera_angle"]), "待确认"),
			firstNonEmpty(reportValueText(values["camera_move"]), "待确认"),
			firstNonEmpty(reportValueText(values["lighting"]), "待确认"),
			firstNonEmpty(reportValueText(values["color"]), "待确认"),
			firstNonEmpty(reportValueText(values["overlay_text"]), reportValueText(values["evidence"]), "待确认"),
			firstNonEmpty(reportValueText(values["confidence"]), "待确认"),
		}
		b.WriteString("<tr>")
		for index, cell := range cells {
			if index == 0 {
				b.WriteString("<th>" + cell + "</th>")
			} else {
				b.WriteString("<td>" + html.EscapeString(cell) + "</td>")
			}
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></div>")
}

func writeStoryboardTable(b *strings.Builder, manifest *videoAnalysisManifest) {
	b.WriteString("<p>连续镜头可拆成多个叙事段落；段落切分不代表原视频存在剪辑。</p><div class=\"table-wrap\"><table class=\"script-table\"><thead><tr><th>镜头 / 段落</th><th>时间 / 时长</th><th>画面还原</th><th>台词 / 字幕</th><th>动作</th><th>机位 / 运镜</th><th>声音</th><th>复刻执行要点（制作建议）</th></tr></thead><tbody>")
	for index, row := range buildVideoAnalysisReport(*manifest).Storyboard {
		cells := []string{fmt.Sprintf("镜头 %d / 段落 %d", row.ShotIndex, index+1), fmt.Sprintf("%.3fs–%.3fs / %.3fs", row.Start, row.End, row.End-row.Start), row.Picture, firstNonEmpty(row.Dialogue, "待确认"), firstNonEmpty(row.Action, "待确认"), firstNonEmpty(row.Camera, "待确认"), firstNonEmpty(row.Sound, "原声待确认"), firstNonEmpty(row.ProductionNotes, "待生成拍摄方案")}
		b.WriteString("<tr>")
		for _, cell := range cells {
			b.WriteString("<td>" + html.EscapeString(cell) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></div>")
	for _, shot := range manifest.Shots {
		values := preferredShotValues(shot)
		if prompt := reportValueText(values["video_prompt"]); prompt != "" {
			b.WriteString(fmt.Sprintf("<div class=\"card\"><h3>镜头 %d · 复刻提示词（制作建议）</h3><p>%s</p></div>", shot.Index, html.EscapeString(prompt)))
		}
		if plan := reportValueText(values["shooting_plan"]); plan != "" {
			b.WriteString(fmt.Sprintf("<p>镜头 %d · 拍摄与布光建议：%s</p>", shot.Index, html.EscapeString(plan)))
		}
	}
}

func preferredShotValues(shot analysisShot) map[string]any {
	values := map[string]any{}
	for _, source := range []map[string]any{shot.Evidence, shot.Vision, shot.Replication, shot.Review} {
		for key, value := range source {
			if value != nil && reportValueText(value) != "" {
				values[key] = value
			}
		}
	}
	return values
}

func shotAnalysisSource(shot analysisShot) string {
	if shot.Review != nil {
		return "Codex/人工复核"
	}
	if shot.Vision != nil {
		return "模型视觉复核"
	}
	if shot.Evidence != nil {
		return "本地证据"
	}
	return ""
}

func writeMissingEvidenceNotice(b *strings.Builder, label, status, reason string) {
	if status == "completed" {
		status = "已完成但没有可显示内容"
	}
	b.WriteString("<div class=\"notice\"><strong>" + html.EscapeString(label) + "不可用：</strong>状态 " + html.EscapeString(firstNonEmpty(status, "unknown")) + "；原因：" + html.EscapeString(firstNonEmpty(reason, "未返回结果")) + "。</div>")
}

func reportValueText(value any) string {
	if value == nil {
		return ""
	}
	if value, ok := value.(string); ok {
		return strings.TrimSpace(value)
	}
	if values, ok := value.([]string); ok {
		return strings.Join(values, "\n")
	}
	if values, ok := value.([]any); ok {
		texts := make([]string, 0, len(values))
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				texts = nil
				break
			}
			texts = append(texts, text)
		}
		if texts != nil {
			return strings.Join(texts, "\n")
		}
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(encoded))
}

func formatAnalysisSeconds(value float64) string {
	if value <= 0 {
		return "待确认"
	}
	return fmt.Sprintf("%.3f 秒", value)
}

func formatAnalysisDimensions(width, height int) string {
	if width <= 0 || height <= 0 {
		return "待确认"
	}
	return fmt.Sprintf("%d × %d", width, height)
}

func formatAnalysisBytes(value int64) string {
	if value <= 0 {
		return "待确认"
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.2f MB（%d bytes）", float64(value)/(unit*unit), value)
}

func analysisAudioPresence(present *bool, status string) string {
	if present == nil {
		return firstNonEmpty(status, "待确认")
	}
	if *present {
		return "有（" + firstNonEmpty(status, "available") + "）"
	}
	return "无（" + firstNonEmpty(status, "no_audio") + "）"
}

func analysisAudioPresenceFromMap(audio map[string]any) string {
	if audio == nil {
		return "待确认"
	}
	present, ok := audio["stream_present"].(bool)
	if !ok {
		return firstNonEmpty(audioStatus(audio), "待确认")
	}
	if present {
		return "有"
	}
	if audioStatus(audio) != "no_audio" {
		return "待确认"
	}
	return "无"
}

func analysisModelLabel(details videoAnalysisDetails) string {
	if details.Model != "" {
		return details.Model
	}
	if details.AnalysisProvider == "codex" {
		return "当前 Codex 会话"
	}
	if details.AnalysisProvider == "local" {
		return "仅提取本地证据"
	}
	return "待确认"
}

func (s *Server) getVideoAnalysisReport(ctx context.Context, _ *mcp.CallToolRequest, input videoAnalysisArtifactInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	// Keep manifest and report reads in the same transaction as review writes.
	s.analysisMu.Lock()
	defer s.analysisMu.Unlock()
	job, err := s.loadAnalysisJobLocked(strings.TrimSpace(input.AnalysisID))
	if err != nil {
		return nil, nil, err
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = "summary"
	}
	if format != "summary" && format != "json" && format != "html" {
		return nil, nil, errors.New("invalid_argument: format must be summary, json, or html")
	}
	ensureVideoAnalysisDetails(&job.Manifest)
	data := map[string]any{"analysis_id": job.ID, "status": job.Manifest.Status, "analysis_provider": analysisProvider(job.Manifest), "review_status": analysisReviewStatus(job.Manifest), "model_status": job.Manifest.ModelStatus, "video_details": job.Manifest.VideoDetails, "local_enrichment_status": job.Manifest.LocalEnrichment.Status, "local_missing_reasons": job.Manifest.LocalEnrichment.MissingReasons, "report_json": job.Manifest.ReportJSON, "report_html": job.Manifest.ReportHTML, "summary": analysisJobSummary(job.Manifest)}
	if format != "summary" {
		if job.Manifest.Status != "completed" {
			return nil, nil, errors.New("analysis_not_ready: reports are available after completion")
		}
		name := "report." + format
		root, err := s.analysisRoot(job.ID, false)
		if err != nil {
			return nil, nil, err
		}
		defer root.Close()
		info, err := root.Stat(name)
		if err != nil || !info.Mode().IsRegular() {
			return nil, nil, analysisStorageError()
		}
		data["artifact_format"] = format
		data["artifact_relative_path"] = analysisReference(job.ID, name)
		data["size_bytes"] = info.Size()
		if info.Size() > 48<<10 {
			data["content_omitted"] = true
		} else {
			content, err := readAnalysisFile(root, name, 48<<10)
			if err != nil {
				return nil, nil, err
			}
			data["content"] = string(content)
			data["content_omitted"] = false
		}
	}
	return nil, envelope("local_video_analysis", "stored", data, job.Manifest.Warnings), nil
}

func (s *Server) getVideoAnalysisShots(ctx context.Context, _ *mcp.CallToolRequest, input videoAnalysisShotsInput) (*mcp.CallToolResult, map[string]any, error) {
	job, err := s.loadAnalysisJob(input.AnalysisID)
	if err != nil {
		return nil, nil, err
	}
	if job.Manifest.Status != "completed" {
		return nil, nil, errors.New("analysis_not_ready: wait until status is completed")
	}
	page := input.Page
	if page == 0 {
		page = 1
	}
	size := input.PageSize
	if size == 0 {
		size = 6
	}
	if page < 1 || size < 1 || size > 12 || input.ImageOffset < 0 {
		return nil, nil, errors.New("invalid_argument: page must be positive and page_size must be between 1 and 12")
	}
	start := len(job.Manifest.Shots)
	if page-1 <= len(job.Manifest.Shots)/size {
		start = (page - 1) * size
	}
	end := start + size
	if end > len(job.Manifest.Shots) {
		end = len(job.Manifest.Shots)
	}
	data := map[string]any{"analysis_id": job.ID, "page": page, "page_size": size, "total": len(job.Manifest.Shots), "has_more": end < len(job.Manifest.Shots), "items": job.Manifest.Shots[start:end], "analysis_provider": analysisProvider(job.Manifest), "review_status": analysisReviewStatus(job.Manifest), "model_status": job.Manifest.ModelStatus, "video_details": job.Manifest.VideoDetails, "local_enrichment_status": job.Manifest.LocalEnrichment.Status, "local_missing_reasons": job.Manifest.LocalEnrichment.MissingReasons}
	result := &mcp.CallToolResult{}
	imageTotal := 0
	for _, shot := range job.Manifest.Shots[start:end] {
		imageTotal += len(shot.Keyframes)
	}
	data["image_offset"], data["images_total"], data["images_returned"], data["images_has_more"], data["next_image_offset"] = input.ImageOffset, imageTotal, 0, false, nil
	if input.IncludeImages {
		root, err := s.analysisRoot(job.ID, false)
		if err != nil {
			return nil, nil, err
		}
		defer root.Close()
		count, position := 0, 0
		for _, shot := range job.Manifest.Shots[start:end] {
			for _, frame := range shot.Keyframes {
				if position < input.ImageOffset {
					position++
					continue
				}
				if count >= 3 {
					break
				}
				raw, readErr := readAnalysisKeyframe(root, job.ID, frame.RelativePath)
				if readErr != nil {
					return nil, nil, readErr
				}
				result.Content = append(result.Content, &mcp.TextContent{Text: fmt.Sprintf("shot=%d timestamp_seconds=%.3f", shot.Index, frame.Timestamp)}, &mcp.ImageContent{MIMEType: "image/jpeg", Data: raw})
				count++
				position++
			}
			if count >= 3 {
				break
			}
		}
		data["images_returned"] = count
		if position < imageTotal {
			data["images_has_more"] = true
			data["next_image_offset"] = position
		}
	}
	if result.Content == nil {
		result.Content = []mcp.Content{&mcp.TextContent{Text: "No keyframe images requested."}}
	}
	return result, envelope("local_video_analysis", "stored", data, job.Manifest.Warnings), nil
}
