package handlers

import "testing"

// 真实响应形状（2026-10-18 从视频号页面 CDP 实抓，仅替换 promoter_key）：
// 两层包装 {code, data:{errCode, data:<WXU 原始返回>}}，
// WXU 原始返回里 objectAsyncInfoMap[].jumpInfo[] 的 businessType==1 那条是商品卡。
func TestParseProductCardResponseRealShape(t *testing.T) {
	body := []byte(`{"code":0,"message":"success","data":{"errCode":0,"errMsg":"ok","ns":"API","method":"finderBatchGetObjectAsyncLoadInfo","data":{"errCode":0,"errMsg":"ok","data":{"baseResponse":{"Ret":0,"ErrMsg":{"String":""}},"objectAsyncInfoMap":[{"objectId":"15032177947323275361","adItemInfo":[],"adLeadInfo":[],"jumpInfo":[{"jumpinfoType":1,"businessType":1,"iconUrl":"https://res.wx.qq.com/shop/public/x.png","wording":"戴可思滋润不油腻宝宝抚触油Z","extInfo":"{\"product_id\":14000842983367,\"shop_nickname\":\"戴可思Dexter\"}","miniAppInfo":{"appId":"wxca8d4b8e8feedc2a","path":"main/pages/productDetail/productDetail.html?productId=10000005998657&realappid=wx2cb5453a4f995e38&promoter_key=FAKE","fetchInfoId":"14000842983367","outProductId":10000005998657}}]}]}}}}`)
	outPID, appid, err := parseProductCardResponse(body, "15032177947323275361")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if outPID != "10000005998657" {
		t.Fatalf("out_product_id = %q, want 10000005998657", outPID)
	}
	if appid != "wx2cb5453a4f995e38" {
		t.Fatalf("real_appid = %q, want wx2cb5453a4f995e38", appid)
	}
}

// outProductId 缺失时必须能从 miniAppInfo.path 的 productId= 兜底。
func TestParseProductCardResponsePathFallback(t *testing.T) {
	body := []byte(`{"code":0,"data":{"errCode":0,"data":{"data":{"objectAsyncInfoMap":[{"objectId":"7","jumpInfo":[{"businessType":1,"miniAppInfo":{"path":"p?productId=123456&realappid=wx2cb5453a4f995e38"}}]}]}}}}`)
	outPID, appid, err := parseProductCardResponse(body, "7")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if outPID != "123456" || appid != "wx2cb5453a4f995e38" {
		t.Fatalf("got outPID=%q appid=%q", outPID, appid)
	}
}

// 作品没挂商品卡（jumpInfo 里没有 businessType==1）。
func TestParseProductCardResponseNoCard(t *testing.T) {
	body := []byte(`{"code":0,"data":{"errCode":0,"data":{"data":{"objectAsyncInfoMap":[{"objectId":"1","jumpInfo":[{"businessType":2,"wording":"直播"}]}]}}}}`)
	if _, _, err := parseProductCardResponse(body, "1"); err == nil {
		t.Fatal("没有商品卡时应返回错误")
	}
}

// 页面没就绪时透传返回 503。
func TestParseProductCardResponsePassthroughError(t *testing.T) {
	body := []byte(`{"code":503,"message":"No ready WeChat page is available."}`)
	if _, _, err := parseProductCardResponse(body, "1"); err == nil {
		t.Fatal("透传失败时应返回错误")
	}
}
