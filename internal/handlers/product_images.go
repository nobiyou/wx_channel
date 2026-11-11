package handlers

// 商品图下载：批量下载视频时，可选地顺带下载该视频挂车商品的**全部主图与规格图**
//
// 数据链路（均为实测已通）：
//  1. 作品 → 商品卡：wx_channel 自己的通用透传 POST /api/v1/channels/api_call
//     （WXU.API.finderBatchGetObjectAsyncLoadInfo），拿到 miniAppInfo.outProductId
//     与 path 里的 realappid。
//  2. 商品 → 主图：橱窗小程序 CGI require('main/common/cgi/productLogic/product')
//     .getProduct({productKey:{productId}, real_wxaproduct_appid})，图片在
//     product.info.head_img[]。这一步只能在小程序渲染进程里跑，所以走本机 CDP 桥
//     （默认 ws://127.0.0.1:62000，可用 SHIPINHAO_MAC_CDP_WS 覆盖）。
//     注意：视频号 Web 页面拿不到商品图 —— WXU.API/API2/API4 里没有任何商品方法。
//  3. 图片落盘到 <视频所在目录>/products/<out_product_id>/head_01.<ext>，
//     扩展名按 Content-Type 决定（实测 head_img 可能是 PNG 但 URL 无后缀）。
//
// 设计约束：商品图是「顺带」的，任何一步失败都只记日志，绝不把视频任务标记为失败。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"wx_channel/internal/utils"
)

const (
	defaultCDPWS       = "ws://127.0.0.1:62000"
	passthroughAPIPath = "/api/v1/channels/api_call"
)

var (
	realAppidRe     = regexp.MustCompile(`realappid=(wx[0-9a-f]{16})`)
	numericIDRe     = regexp.MustCompile(`^\d{1,32}$`)
	pathProductRe   = regexp.MustCompile(`[?&]productId=(\d{1,32})`)
	talentAppidRe   = regexp.MustCompile(`^wx[0-9a-f]{16}$`)
	productImageExt = map[string]string{
		"image/jpeg": ".jpg",
		"image/jpg":  ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
		"image/gif":  ".gif",
	}
)

func cdpWSURL() string {
	if v := strings.TrimSpace(os.Getenv("WX_CHANNEL_CDP_WS")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("SHIPINHAO_MAC_CDP_WS")); v != "" {
		return v
	}
	return defaultCDPWS
}

// productDetailExpr 与 tools/video_product_probe.py 的 PRODUCT_DETAIL_EXPR 等价。
// __PID__ = out_product_id，__APPID2__ = 商品所属 realappid。
const productDetailExpr = `
(async () => {
  const out = { ok: false };
  let P;
  try {
    const M = require('main/common/cgi/productLogic/product/index');
    P = M && M.default && typeof M.default === 'object' ? Object.assign({}, M.default, M) : M;
  } catch (e) { out.message = 'require productLogic 失败: ' + String(e).slice(0, 150); return out; }
  if (typeof P.getProduct !== 'function') { out.message = 'getProduct 不存在'; return out; }
  const pk = { productId: '__PID__' };
  const extraAppid = '__APPID2__';
  const req = { productKey: pk, noNeedCache: true, isGetEditData: false };
  if (extraAppid) req.real_wxaproduct_appid = extraAppid;
  let r;
  try {
    r = await Promise.race([
      P.getProduct(req),
      new Promise((_, rej) => setTimeout(() => rej(new Error('timeout 25s')), 25000))]);
  } catch (e) { out.message = String((e && e.message) || e).slice(0, 200); return out; }
  let raw = null;
  try { raw = JSON.parse((r && r.rawData) || (r && r.respData && r.respData.data) || '{}'); } catch (e) {}
  if (!raw) { out.message = 'rawData 解析失败'; return out; }
  const prod = raw.product || {};
  const info = prod.info || {};
  out.ok = !!(info.title || (info.head_img && info.head_img.length));
  out.errcode = raw.errcode;
  if (!out.ok) out.message = 'product 为空（appid 与商品不匹配？）';
  out.title = info.title;
  out.short_title = info.short_title;
  out.head_imgs = Array.isArray(info.head_img) ? info.head_img : [];
  // 规格图：product_sale_attrs[].items[].img_url（每个销售规格一张图）
  const attrs = Array.isArray(info.product_sale_attrs) ? info.product_sale_attrs : [];
  const specs = [];
  for (const a of attrs) {
    for (const it of (a.items || [])) {
      if (it && it.img_url) specs.push({ url: it.img_url, key: a.key || '', value: it.value || '' });
    }
  }
  out.specs = specs;
  return out;
})()
`

// downloadProductImages 下载该任务挂车商品的主图（每商品一张）。失败只记日志。
func (h *BatchHandler) downloadProductImages(ctx context.Context, task *BatchTask, videoPath string) {
	if !task.DownloadProductImages || ctx.Err() != nil {
		return
	}
	if !numericIDRe.MatchString(strings.TrimSpace(task.ID)) {
		utils.Warn("🖼️ [商品图] 跳过（作品 ID 非法）: %s", task.Title)
		return
	}

	outPID, appid, err := h.fetchProductCard(ctx, strings.TrimSpace(task.ID))
	if err != nil {
		utils.Warn("🖼️ [商品图] 跳过（未拿到商品卡）: %s - %v", task.Title, err)
		return
	}

	imgs, specs, title, err := cdpProductImages(ctx, outPID, appid)
	if err != nil {
		utils.Warn("🖼️ [商品图] 跳过（商品详情失败）: %s - %v", task.Title, err)
		return
	}
	if len(imgs) == 0 && len(specs) == 0 {
		utils.Warn("🖼️ [商品图] 跳过（商品没有图片）: %s - %s", task.Title, title)
		return
	}

	dir := productImageDir(videoPath, outPID, title)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		utils.Warn("🖼️ [商品图] 建目录失败: %s - %v", task.Title, err)
		return
	}

	headSlug := productSlug(title, 20)
	savedHead, headErr := downloadImageFiles(ctx, imgs, dir, "head", func(int) string { return headSlug })
	if headErr != nil {
		utils.Warn("🖼️ [商品图] 主图下载失败: %s - %v", task.Title, headErr)
	}
	specURLs := make([]string, 0, len(specs))
	for _, sp := range specs {
		specURLs = append(specURLs, sp.URL)
	}
	savedSpec, specErr := downloadImageFiles(ctx, specURLs, dir, "spec", func(i int) string { return productSlug(specs[i].Value, 20) })
	if specErr != nil {
		utils.Warn("🖼️ [商品图] 规格图下载失败: %s - %v", task.Title, specErr)
	}
	all := append(append([]string{}, savedHead...), savedSpec...)
	if len(all) == 0 {
		utils.Warn("🖼️ [商品图] 一张都没下成: %s - %s", task.Title, title)
		return
	}
	task.ProductImagePath = strings.Join(all, ",")
	if err := writeProductMeta(dir, outPID, appid, title, savedHead, specs, savedSpec); err != nil {
		utils.Warn("🖼️ [商品图] 元数据写入失败: %v", err)
	}
	utils.Info("🖼️ [商品图] 完成: %s → 主图 %d 张 + 规格图 %d 张（商品: %s）", task.Title, len(savedHead), len(savedSpec), title)
}

// fetchProductCard 经 wx_channel 通用透传查「作品 → 挂车商品卡」。
func (h *BatchHandler) fetchProductCard(ctx context.Context, objectID string) (outPID, appid string, err error) {
	port := 2025
	if cfg := h.getConfig(); cfg != nil && cfg.Port > 0 {
		port = cfg.Port
	}
	payload := map[string]any{
		"ns":     "API",
		"method": "finderBatchGetObjectAsyncLoadInfo",
		"args": []any{map[string]any{
			"object_uid_list": []any{map[string]any{"object_id": objectID}},
			"scene":           0,
		}},
		"timeout_seconds": 45,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d%s", port, passthroughAPIPath), bytes.NewReader(buf))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 55 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("透传请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("透传 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))[:min(160, len(body))])
	}

	return parseProductCardResponse(body, objectID)
}

// parseProductCardResponse 解析透传返回，返回 out_product_id 与 realappid。
func parseProductCardResponse(body []byte, objectID string) (outPID, appid string, err error) {
	// WXU 原始返回：{errCode, errMsg, data:{baseResponse, objectAsyncInfoMap:[...]}}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ErrCode int             `json:"errCode"`
			ErrMsg  string          `json:"errMsg"`
			Data    json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("透传响应解析失败: %w", err)
	}
	if env.Code != 0 {
		return "", "", fmt.Errorf("透传 code=%d %s", env.Code, strings.TrimSpace(env.Message))
	}
	if env.Data.ErrCode != 0 {
		return "", "", fmt.Errorf("页面侧 errCode=%d %s", env.Data.ErrCode, strings.TrimSpace(env.Data.ErrMsg))
	}
	var inner struct {
		Data struct {
			ObjectAsyncInfoMap []map[string]json.RawMessage `json:"objectAsyncInfoMap"`
		} `json:"data"`
	}
	if err := json.Unmarshal(env.Data.Data, &inner); err != nil {
		return "", "", fmt.Errorf("objectAsyncInfoMap 解析失败: %w", err)
	}
	if len(inner.Data.ObjectAsyncInfoMap) == 0 {
		return "", "", fmt.Errorf("该作品没有挂商品卡")
	}

	for _, entry := range inner.Data.ObjectAsyncInfoMap {
		var oid string
		if raw, ok := entry["objectId"]; ok {
			_ = json.Unmarshal(raw, &oid)
		}
		if oid != objectID {
			continue
		}
		var jumps []map[string]json.RawMessage
		if raw, ok := entry["jumpInfo"]; ok {
			_ = json.Unmarshal(raw, &jumps)
		}
		for _, j := range jumps {
			var businessType int
			if raw, ok := j["businessType"]; ok {
				_ = json.Unmarshal(raw, &businessType)
			} else if raw, ok := j["business_type"]; ok {
				_ = json.Unmarshal(raw, &businessType)
			}
			if businessType != 1 {
				continue
			}
			var mini struct {
				OutProductID any    `json:"outProductId"`
				Path         string `json:"path"`
			}
			if raw, ok := j["miniAppInfo"]; ok {
				_ = json.Unmarshal(raw, &mini)
			} else if raw, ok := j["mini_app_info"]; ok {
				_ = json.Unmarshal(raw, &mini)
			}
			pid := anyIDToString(mini.OutProductID)
			// 兜底：实测 miniAppInfo 一定有 outProductId，但 path 里的 productId 是同一个值。
			if pid == "" {
				if m := pathProductRe.FindStringSubmatch(mini.Path); len(m) > 1 {
					pid = m[1]
				}
			}
			if pid == "" {
				continue
			}
			if m := realAppidRe.FindStringSubmatch(mini.Path); len(m) > 1 {
				appid = m[1]
			}
			return pid, appid, nil
		}
	}
	return "", "", fmt.Errorf("商品卡里没有 business_type=1 的条目")
}

func anyIDToString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

// --- CDP 客户端（最小实现：attach AppIndex → 找 require context → Runtime.evaluate）---

type cdpConn struct {
	ws     *websocket.Conn
	mu     sync.Mutex
	nextID int64
}

func dialCDP(ctx context.Context) (*cdpConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(dialCtx, cdpWSURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("连不上 CDP 桥 %s: %w", cdpWSURL(), err)
	}
	return &cdpConn{ws: ws}, nil
}

func (c *cdpConn) close() {
	if c == nil || c.ws == nil {
		return
	}
	_ = c.ws.Close(websocket.StatusNormalClosure, "")
}

// call 串行发一条 CDP 命令并等对应 id 的响应；其它事件/无关响应直接丢弃。
func (c *cdpConn) call(ctx context.Context, method string, params any, sessionID string, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++
	id := c.nextID
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	buf, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	writeCtx, cancelWrite := context.WithTimeout(ctx, timeout)
	err = c.ws.Write(writeCtx, websocket.MessageText, buf)
	cancelWrite()
	if err != nil {
		return nil, fmt.Errorf("CDP 写入失败(%s): %w", method, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("CDP 超时(%s)", method)
		}
		readCtx, cancelRead := context.WithDeadline(ctx, deadline)
		_, data, err := c.ws.Read(readCtx)
		cancelRead()
		if err != nil {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("CDP 超时(%s)", method)
			}
			return nil, fmt.Errorf("CDP 读取失败(%s): %w", method, err)
		}
		var resp struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("CDP %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// productSpec 是商品的一个销售规格：规格名 + 规格值 + 规格图。
type productSpec struct {
	URL   string `json:"url"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// cdpProductImages 在小程序里调商品详情，返回全部主图、规格图与商品标题。
// cdpMu 串行化商品图 CDP 调用：批量 worker 并发 5，但 WMPF 桥的 AppIndex 会话
// 从未被测过并发 attach，排队最稳（单条约 1-2s，商品图不是关键路径）。
var cdpMu sync.Mutex

func cdpProductImages(ctx context.Context, outProductID, appid string) ([]string, []productSpec, string, error) {
	cdpMu.Lock()
	defer cdpMu.Unlock()
	if !numericIDRe.MatchString(outProductID) {
		return nil, nil, "", fmt.Errorf("out_product_id 非法: %s", outProductID)
	}
	if appid != "" && !talentAppidRe.MatchString(appid) {
		return nil, nil, "", fmt.Errorf("appid 非法: %s", appid)
	}

	conn, err := dialCDP(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	defer conn.close()

	targetsRaw, err := conn.call(ctx, "Target.getTargets", map[string]any{}, "", 10*time.Second)
	if err != nil {
		return nil, nil, "", err
	}
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			URL      string `json:"url"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(targetsRaw, &targets); err != nil {
		return nil, nil, "", err
	}
	targetID := ""
	for _, t := range targets.TargetInfos {
		if t.Type == "page" && strings.Contains(t.URL, "page-frame.html") {
			targetID = t.TargetID
			break
		}
	}
	if targetID == "" {
		return nil, nil, "", fmt.Errorf("没有 AppIndex 目标：橱窗小程序没打开？")
	}

	attachRaw, err := conn.call(ctx, "Target.attachToTarget",
		map[string]any{"targetId": targetID, "flatten": true}, "", 15*time.Second)
	if err != nil {
		return nil, nil, "", err
	}
	var attach struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachRaw, &attach); err != nil || attach.SessionID == "" {
		return nil, nil, "", fmt.Errorf("attach 失败")
	}

	// Runtime.enable 失败不影响后续 evaluate。
	_, _ = conn.call(ctx, "Runtime.enable", map[string]any{}, attach.SessionID, 10*time.Second)

	// 找 require 可用的 execution context（小程序里通常是 3，但别写死）。
	ctxID := 0
	for i := 1; i <= 8; i++ {
		raw, err := conn.call(ctx, "Runtime.evaluate", map[string]any{
			"expression": "typeof require", "returnByValue": true, "contextId": i,
		}, attach.SessionID, 8*time.Second)
		if err != nil {
			continue
		}
		var r struct {
			Result struct {
				Value any `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &r) == nil {
			if s, ok := r.Result.Value.(string); ok && s == "function" {
				ctxID = i
				break
			}
		}
	}
	if ctxID == 0 {
		return nil, nil, "", fmt.Errorf("找不到 require 可用的 context（橱窗小程序未就绪？）")
	}

	expr := strings.ReplaceAll(productDetailExpr, "__PID__", outProductID)
	expr = strings.ReplaceAll(expr, "__APPID2__", appid)
	raw, err := conn.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true, "awaitPromise": true, "contextId": ctxID,
	}, attach.SessionID, 90*time.Second)
	if err != nil {
		return nil, nil, "", err
	}
	// conn.call 返回的已经是 CDP 响应的 result 字段，
	// 形状 {"result":{"type":"object","value":<JS 返回值>}}，只需再取一层。
	var outer struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, nil, "", err
	}
	var detail struct {
		OK       bool          `json:"ok"`
		Message  string        `json:"message"`
		Title    string        `json:"title"`
		HeadImgs []string      `json:"head_imgs"`
		Specs    []productSpec `json:"specs"`
	}
	if err := json.Unmarshal(outer.Result.Value, &detail); err != nil {
		return nil, nil, "", fmt.Errorf("商品详情返回解析失败: %w", err)
	}
	if !detail.OK {
		return nil, nil, "", fmt.Errorf("商品详情失败: %s", detail.Message)
	}
	return detail.HeadImgs, detail.Specs, detail.Title, nil
}

// productSlug 把商品标题压成可用作文件名的短标识：去路径非法字符、压空白、按 runes 截断。
func productSlug(title string, maxRunes int) string {
	s := strings.TrimSpace(title)
	s = strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	rs := []rune(s)
	if len(rs) > maxRunes {
		rs = rs[:maxRunes]
	}
	// 截断可能停在分隔符上，收尾去掉 _ 与空格
	return strings.Trim(strings.TrimSpace(string(rs)), "_ ")
}

// productImageDir 返回商品图目录：products/<out_product_id>_<商品名>。
func productImageDir(videoPath, outProductID, title string) string {
	name := outProductID
	if slug := productSlug(title, 40); slug != "" {
		name = outProductID + "_" + slug
	}
	return filepath.Join(filepath.Dir(videoPath), "products", name)
}

// writeProductMeta 在商品图目录落一份 product.json，避免只看到一串商品 id。
func writeProductMeta(dir, outPID, appid, title string, headFiles []string, specs []productSpec, specFiles []string) error {
	headNames := make([]string, 0, len(headFiles))
	for _, f := range headFiles {
		headNames = append(headNames, filepath.Base(f))
	}
	specNames := make([]string, 0, len(specFiles))
	for _, f := range specFiles {
		specNames = append(specNames, filepath.Base(f))
	}
	type specMeta struct {
		Key   string `json:"key"`
		Value string `json:"value"`
		File  string `json:"file"`
	}
	sms := make([]specMeta, 0, len(specs))
	for i, sp := range specs {
		f := ""
		if i < len(specFiles) {
			f = filepath.Base(specFiles[i])
		}
		sms = append(sms, specMeta{Key: sp.Key, Value: sp.Value, File: f})
	}
	all := append(append([]string{}, headNames...), specNames...)
	meta := struct {
		OutProductID string     `json:"out_product_id"`
		RealAppID    string     `json:"real_appid"`
		Title        string     `json:"title"`
		HeadImages   int        `json:"head_images"`
		SpecImages   int        `json:"spec_images"`
		Specs        []specMeta `json:"specs,omitempty"`
		Files        []string   `json:"files"`
		DownloadedAt string     `json:"downloaded_at"`
	}{
		OutProductID: outPID,
		RealAppID:    appid,
		Title:        title,
		HeadImages:   len(headNames),
		SpecImages:   len(specNames),
		Specs:        sms,
		Files:        all,
		DownloadedAt: time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "product.json"), append(b, '\n'), 0o644)
}

// existingProductImage 找目录里已存在的 <prefix>_NN_* 文件。
// 不用 filepath.Glob：商品名/规格值里可能有 [ ] * ? 等 glob 元字符，会把匹配搞坏。
func existingProductImage(dir, prefix string, idx int) string {
	want := fmt.Sprintf("%s_%02d_", prefix, idx)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), want) || strings.HasSuffix(e.Name(), ".part") {
			continue
		}
		return filepath.Join(dir, e.Name())
	}
	return ""
}

// downloadImageFiles 下载一组图片到 dir，命名 <prefix>_NN[_<slug>].<ext>。
// 已存在的序号直接复用（断点续跑）；单张失败只记日志，不中断其余。
func downloadImageFiles(ctx context.Context, urls []string, dir, prefix string, slugOf func(int) string) ([]string, error) {
	saved := make([]string, 0, len(urls))
	var lastErr error
	for i, u := range urls {
		if ctx.Err() != nil {
			return saved, ctx.Err()
		}
		idx := i + 1
		if existing := existingProductImage(dir, prefix, idx); existing != "" {
			saved = append(saved, existing)
			continue
		}
		slug := ""
		if slugOf != nil {
			slug = slugOf(i)
		}
		dest, err := downloadOneImage(ctx, u, dir, prefix, idx, slug)
		if err != nil {
			lastErr = err
			utils.Warn("🖼️ [商品图] %s 第 %d 张失败: %v", prefix, idx, err)
			continue
		}
		saved = append(saved, dest)
	}
	if len(saved) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return saved, nil
}

func downloadOneImage(ctx context.Context, imgURL, dir, prefix string, idx int, slug string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, imgURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("图片 HTTP %d", resp.StatusCode)
	}
	ext := productImageExt[strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))]
	if ext == "" {
		ext = ".jpg"
	}
	base := fmt.Sprintf("%s_%02d", prefix, idx)
	if slug != "" {
		base += "_" + slug
	}
	dest := filepath.Join(dir, base+ext)
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dest, nil
}
