function FindProxyForURL(url, host) {
    // WeChat Video Channel related domains only
    if (shExpMatch(host, "channels.weixin.qq.com") ||
        shExpMatch(host, "res.wx.qq.com") ||
        shExpMatch(host, "finder.video.qq.com") ||
        shExpMatch(host, "szextshort.weixin.qq.com") ||
        shExpMatch(host, "szshort.weixin.qq.com") ||
        shExpMatch(host, "*.wximg.cn") ||
        shExpMatch(host, "wpimg.wallstcn.com") ||
        shExpMatch(host, "finder.video.qq.com")) {
        return "PROXY 127.0.0.1:__PORT__";
    }
    // Everything else: DIRECT (Clash TUN handles it)
    return "DIRECT";
}
