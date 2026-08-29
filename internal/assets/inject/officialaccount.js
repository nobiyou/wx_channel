(function () {
  "use strict";

  var config = window.__wx_channels_mp_config__ || {};
  var submitted = {};
  var metricSubmitted = {};
  var articleCommentRequestStarted = false;
  var pageMetricData = {};
  var pageMetricAliasPriority = {};
  var pageMediaObjects = [];
  var pageVideoPageInfoObjects = [];
  var pageVideoPageInfos = [];
  var pageVideoTransferObjects = [];
  var panelId = "__wx_channels_official_account__";
  var toolsRootId = "__wx_channels_mp_tools__";
  var messageListDialogId = "__wx_channels_mp_message_list__";
  var noticeId = "__wx_channels_mp_notice__";
  var articleDownloadPromise = null;
  var articleMenuIcons = {
    copy: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="10" height="10" rx="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>',
    rss: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 11a9 9 0 0 1 9 9"></path><path d="M4 4a16 16 0 0 1 16 16"></path><circle cx="5" cy="19" r="1"></circle></svg>',
    list: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"></line><line x1="8" y1="12" x2="21" y2="12"></line><line x1="8" y1="18" x2="21" y2="18"></line><line x1="3" y1="6" x2="3.01" y2="6"></line><line x1="3" y1="12" x2="3.01" y2="12"></line><line x1="3" y1="18" x2="3.01" y2="18"></line></svg>',
    download: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"></path><path d="m7 10 5 5 5-5"></path><path d="M5 21h14"></path></svg>',
    console: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2"></rect><path d="M7 8h10"></path><path d="M7 12h4"></path><path d="M7 16h7"></path><circle cx="17" cy="12" r="1"></circle></svg>',
    archive: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16"></path><path d="M5 7v12h14V7"></path><path d="M3 4h18v3H3z"></path><path d="M10 11h4"></path></svg>',
    close: '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"></path><path d="m6 6 12 12"></path></svg>'
  };
  var maxCaptureAttempts = 120;
  var pageDataFields = [
    "bizuin",
    "user_name",
    "nick_name",
    "round_head_img",
    "hd_head_img",
    "author_id",
    "user_uin",
    "uin",
    "key",
    "pass_ticket",
    "appmsg_token",
    "link"
  ];

  function firstValue() {
    for (var i = 0; i < arguments.length; i += 1) {
      var value = arguments[i];
      if (value !== undefined && value !== null && String(value).trim() !== "") {
        return String(value).trim();
      }
    }
    return "";
  }

  function objectValue(object, key) {
    return object && typeof object === "object" ? object[key] : "";
  }

  function urlValue(rawURL, name) {
    try {
      return new URL(rawURL, window.location.href).searchParams.get(name) || "";
    } catch (error) {
      return "";
    }
  }

  function isPageDataObject(value) {
    if (!value || typeof value !== "object") {
      return false;
    }
    for (var i = 0; i < pageDataFields.length; i += 1) {
      if (objectValue(value, pageDataFields[i]) !== undefined && objectValue(value, pageDataFields[i]) !== null) {
        return true;
      }
    }
    return false;
  }

  function mergePageData(value) {
    if (!isPageDataObject(value)) {
      return false;
    }
    if (!window.cgiDataNew || typeof window.cgiDataNew !== "object") {
      window.cgiDataNew = {};
    }
    var changed = false;
    for (var i = 0; i < pageDataFields.length; i += 1) {
      var key = pageDataFields[i];
      var nextValue = objectValue(value, key);
      if (nextValue === undefined || nextValue === null || String(nextValue).trim() === "") {
        continue;
      }
      if (window.cgiDataNew[key] !== nextValue) {
        window.cgiDataNew[key] = nextValue;
        changed = true;
      }
    }
    return changed;
  }

  var metricAliases = {
    view_count: ["read_num3", "readnum3", "read_num2", "readnum2", "read_num", "readnum", "reading_count", "readingcount", "view_count", "viewcount", "read_count", "readcount"],
    like_count: ["like_num", "likenum", "like_count", "likecount", "old_like_num", "oldlikenum", "old_like_count", "praise_count", "praisecount", "digg_count", "diggcount"],
    comment_count: ["elected_comment_total_cnt", "electedcommenttotalcnt", "preload_comment_total_cnt", "preloadcommenttotalcnt", "appmsg_comment_total_cnt", "appmsgcommenttotalcnt", "comment_total_cnt", "commenttotalcnt", "comments_count", "commentscount", "comment_count", "commentcount", "comment_num", "commentnum", "cmt_count", "cmtcount"],
    share_count: ["share_count", "sharecount", "share_num", "sharenum", "forward_count", "forwardcount", "repost_count", "repostcount", "repost_num", "repostnum", "forward_num", "forwardnum"],
    collect_count: ["collect_count", "collectcount", "collect_num", "collectnum", "favorite_count", "favoritecount", "favorite_num", "favoritenum", "fav_count", "favcount", "fav_num", "favnum"],
    reward_count: ["reward_count", "rewardcount", "reward_num", "rewardnum", "rewards_count", "rewardscount", "appmsg_reward_count", "appmsgrewardcount", "tip_count", "tipcount"]
  };

  var metricLabels = {
    view_count: ["阅读量", "阅读", "浏览量", "浏览"],
    like_count: ["点赞数", "点赞", "喜欢"],
    comment_count: ["评论数", "评论", "留言"],
    share_count: ["转发数", "转发", "分享数", "分享"],
    collect_count: ["收藏数", "收藏"],
    reward_count: ["赞赏数", "赞赏", "打赏数", "打赏"]
  };

  var mediaIdentityFields = [
    "video_id",
    "videoId",
    "vid",
    "mpvid",
    "media_id",
    "mediaId",
    "audio_fileid",
    "audioFileID",
    "voice_encode_fileid",
    "voiceEncodeFileID"
  ];
  var mediaTransferFields = [
    "duration_ms",
    "durationMs",
    "duration",
    "video_duration",
    "videoDuration",
    "video_play_len",
    "videoPlayLen",
    "play_url",
    "playUrl",
    "video_url",
    "videoUrl",
    "url",
    "format_id",
    "formatId",
    "video_quality_level",
    "videoQualityLevel",
    "width",
    "height"
  ];

  function metricNumber(value) {
    if (typeof value === "number") {
      return isFinite(value) && value >= 0 ? Math.round(value) : null;
    }
    if (value === undefined || value === null) {
      return null;
    }
    var text = String(value).replace(/,/g, "").trim();
    var match = text.match(/[-+]?\d[\d ]*(?:\.\d+)?[ \t\r\n]*(?:万|千|亿|w|k|m)?/i);
    if (!match) {
      return null;
    }
    var raw = match[0].replace(/[ \t\r\n]/g, "");
    var multiplier = 1;
    var lower = raw.toLowerCase();
    if (lower.slice(-1) === "万" || lower.slice(-1) === "w") {
      multiplier = 10000;
      raw = raw.slice(0, -1);
    } else if (lower.slice(-1) === "千" || lower.slice(-1) === "k") {
      multiplier = 1000;
      raw = raw.slice(0, -1);
    } else if (lower.slice(-1) === "亿") {
      multiplier = 100000000;
      raw = raw.slice(0, -1);
    } else if (lower.slice(-1) === "m") {
      multiplier = 1000000;
      raw = raw.slice(0, -1);
    }
    var number = Number(raw) * multiplier;
    return isFinite(number) && number >= 0 ? Math.round(number) : null;
  }

  function metricKeyMatches(key, aliases) {
    var normalized = String(key || "").toLowerCase();
    for (var i = 0; i < aliases.length; i += 1) {
      if (normalized === aliases[i]) {
        return true;
      }
    }
    return false;
  }

  function metricAliasPriority(name, key) {
    var aliases = metricAliases[name] || [];
    var normalized = String(key || "").toLowerCase();
    for (var i = 0; i < aliases.length; i += 1) {
      if (normalized === aliases[i]) {
        return aliases.length - i;
      }
    }
    return 0;
  }

  function mergeMetricData(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return false;
    }
    var changed = false;
    Object.keys(value).forEach(function (key) {
      Object.keys(metricAliases).forEach(function (name) {
        if (!metricKeyMatches(key, metricAliases[name])) {
          return;
        }
        var parsed = metricNumber(value[key]);
        var priority = metricAliasPriority(name, key);
        var existingPriority = pageMetricAliasPriority[name] || 0;
        if (parsed === null || priority < existingPriority ||
          (priority === existingPriority && pageMetricData[name] === parsed)) {
          return;
        }
        pageMetricData[name] = parsed;
        pageMetricAliasPriority[name] = priority;
        changed = true;
      });
    });
    return changed;
  }

  function normalizedMediaKey(key) {
    return String(key || "").toLowerCase().replace(/[\s_-]/g, "");
  }

  function mediaContainerKind(key) {
    switch (normalizedMediaKey(key)) {
      case "videopageinfo":
        return "video_page_info";
      case "videopageinfos":
      case "videos":
        return "video_page_infos";
      case "mpvideotransinfo":
        return "mp_video_trans_info";
      case "videoinfo":
      case "mediainfo":
      case "video":
      case "audio":
      case "audioinfo":
        return "media_object";
      default:
        return "";
    }
  }

  function mediaObjectHasValue(value, fields) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return false;
    }
    for (var i = 0; i < fields.length; i += 1) {
      var candidate = objectValue(value, fields[i]);
      if (candidate !== undefined && candidate !== null && String(candidate).trim() !== "") {
        return true;
      }
    }
    return false;
  }

  function looksLikeMediaObject(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return false;
    }
    if (mediaObjectHasValue(value, mediaIdentityFields)) {
      return true;
    }
    var hasDuration = mediaObjectHasValue(value, [
      "duration_ms",
      "durationMs",
      "duration",
      "video_duration",
      "videoDuration",
      "video_play_len",
      "videoPlayLen"
    ]);
    var hasAudio = mediaObjectHasValue(value, [
      "audio_fileid",
      "audioFileID",
      "audioFileId",
      "voice_encode_fileid",
      "voiceEncodeFileID"
    ]);
    var hasPlayback = mediaObjectHasValue(value, [
      "play_url",
      "playUrl",
      "video_url",
      "videoUrl",
      "media_url",
      "mediaUrl"
    ]);
    var hasURL = mediaObjectHasValue(value, ["url"]);
    var hasFormat = mediaObjectHasValue(value, [
      "format_id",
      "formatId",
      "video_quality_level",
      "videoQualityLevel",
      "width",
      "height"
    ]);
    return hasDuration || hasAudio || hasPlayback || (hasURL && hasFormat);
  }

  function pushUniqueMediaObject(list, value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return;
    }
    if (list.indexOf(value) < 0) {
      list.push(value);
    }
  }

  function mergeMediaData(value, fieldName) {
    if (!value || typeof value !== "object") {
      return false;
    }
    var kind = mediaContainerKind(fieldName);
    var changed = false;
    if (Array.isArray(value)) {
      for (var i = 0; i < value.length; i += 1) {
        var item = value[i];
        if (!item || typeof item !== "object" || Array.isArray(item)) {
          continue;
        }
        var before = pageMediaObjects.length;
        mergeMediaData(item, fieldName);
        changed = changed || pageMediaObjects.length !== before;
      }
      return changed;
    }
    if (kind === "video_page_info") {
      pushUniqueMediaObject(pageVideoPageInfoObjects, value);
    } else if (kind === "video_page_infos") {
      pushUniqueMediaObject(pageVideoPageInfos, value);
    } else if (kind === "mp_video_trans_info") {
      pushUniqueMediaObject(pageVideoTransferObjects, value);
    }
    if (kind || looksLikeMediaObject(value)) {
      var lengthBefore = pageMediaObjects.length;
      pushUniqueMediaObject(pageMediaObjects, value);
      changed = pageMediaObjects.length !== lengthBefore;
    }
    return changed;
  }

  function inspectNetworkValue(value, depth, fieldName) {
    if (depth > 12 || value === undefined || value === null) {
      return;
    }
    if (typeof value === "string") {
      var text = value.trim();
      if (!text || text.length > 2 * 1024 * 1024) {
        return;
      }
      try {
        inspectNetworkValue(JSON.parse(text), depth + 1, fieldName);
      } catch (error) {
        // HTML and non-JSON responses are already covered by page polling.
      }
      return;
    }
    if (Array.isArray(value)) {
      mergeMediaData(value, fieldName);
      for (var i = 0; i < Math.min(value.length, 100); i += 1) {
        inspectNetworkValue(value[i], depth + 1, fieldName);
      }
      return;
    }
    if (typeof value !== "object") {
      return;
    }

    mergeMediaData(value, fieldName);
    mergePageData(value.cgiDataNew);
    mergePageData(value.cgi_data_new);
    mergePageData(value);
    mergeMetricData(value);
    Object.keys(value).slice(0, 256).forEach(function (key) {
      if (value[key] !== undefined && value[key] !== null) {
        inspectNetworkValue(value[key], depth + 1, key);
      }
    });
  }

  function networkURL(input) {
    if (typeof input === "string") {
      return input;
    }
    if (input && input.url) {
      return input.url;
    }
    return String(input || "");
  }

  function isOfficialNetworkURL(rawURL) {
    try {
      var parsed = new URL(networkURL(rawURL), window.location.href);
      var path = parsed.pathname || "";
      return parsed.hostname.toLowerCase() === "mp.weixin.qq.com" &&
        (path === "/s" || path.indexOf("/s/") === 0 ||
          path === "/mp/profile_ext" || path === "/mp/author" || path === "/mp/getappmsgext" || path === "/mp/appmsg_comment");
    } catch (error) {
      return false;
    }
  }

  function inspectNetworkResponse(rawURL, response) {
    if (!isOfficialNetworkURL(rawURL) || !response || typeof response.clone !== "function") {
      return;
    }
    try {
      var copy = response.clone();
      if (copy && typeof copy.text === "function") {
        copy.text().then(function (body) {
          inspectNetworkValue(body, 0);
        }).catch(function () {
          // Ignore an unreadable clone and keep the original response intact.
        });
      }
    } catch (error) {
      // Some WebView response objects do not support clone().
    }
  }

  function installFetchObserver() {
    var marker = "__wx_channels_mp_fetch_observer__";
    if (window[marker] || typeof window.fetch !== "function") {
      return;
    }
    var originalFetch = window.fetch;
    window.fetch = function () {
      var rawURL = networkURL(arguments[0]);
      var result = originalFetch.apply(this, arguments);
      return Promise.resolve(result).then(function (response) {
        inspectNetworkResponse(rawURL, response);
        return response;
      });
    };
    window[marker] = true;
  }

  function installXHRObserver() {
    var XHR = window.XMLHttpRequest;
    if (!XHR || !XHR.prototype || XHR.prototype.__wx_channels_mp_xhr_observer__) {
      return;
    }
    var prototype = XHR.prototype;
    var originalOpen = prototype.open;
    var originalSend = prototype.send;
    if (typeof originalOpen !== "function" || typeof originalSend !== "function") {
      return;
    }
    prototype.open = function () {
      this.__wx_channels_mp_request_url__ = arguments[1];
      return originalOpen.apply(this, arguments);
    };
    prototype.send = function () {
      var xhr = this;
      var observe = function () {
        if (xhr.readyState !== 4 || !isOfficialNetworkURL(xhr.__wx_channels_mp_request_url__)) {
          return;
        }
        try {
          if (xhr.responseType === "json") {
            inspectNetworkValue(xhr.response, 0);
          } else if (!xhr.responseType || xhr.responseType === "text") {
            inspectNetworkValue(xhr.responseText, 0);
          }
        } catch (error) {
          // Ignore responseType restrictions imposed by the WebView.
        }
      };
      if (typeof xhr.addEventListener === "function") {
        xhr.addEventListener("load", observe);
      }
      return originalSend.apply(this, arguments);
    };
    prototype.__wx_channels_mp_xhr_observer__ = true;
  }

  function installNetworkObservers() {
    installFetchObserver();
    installXHRObserver();
  }

  function elementText(selectors) {
    if (!document || typeof document.querySelector !== "function") {
      return "";
    }
    for (var i = 0; i < selectors.length; i += 1) {
      try {
        var element = document.querySelector(selectors[i]);
        var value = element && (element.textContent || element.innerText);
        if (value && String(value).trim() !== "") {
          return String(value).trim();
        }
      } catch (error) {
        // Keep trying the other page layouts.
      }
    }
    return "";
  }

  function elementAttribute(selectors, attributes) {
    if (!document || typeof document.querySelector !== "function") {
      return "";
    }
    for (var i = 0; i < selectors.length; i += 1) {
      try {
        var element = document.querySelector(selectors[i]);
        if (!element || typeof element.getAttribute !== "function") {
          continue;
        }
        for (var j = 0; j < attributes.length; j += 1) {
          var value = element.getAttribute(attributes[j]);
          if (value && String(value).trim() !== "") {
            return String(value).trim();
          }
        }
      } catch (error) {
        // Keep trying the other page layouts.
      }
    }
    return "";
  }

  function authorIDFromLink() {
    if (!document || typeof document.querySelectorAll !== "function") {
      return "";
    }
    try {
      var links = document.querySelectorAll('a[href*="author_id="], [data-author-id]');
      for (var i = 0; i < links.length; i += 1) {
        var dataAuthorID = links[i].getAttribute && links[i].getAttribute("data-author-id");
        if (dataAuthorID) {
          return String(dataAuthorID).trim();
        }
        var href = links[i].getAttribute && links[i].getAttribute("href");
        if (href) {
          var parsed = new URL(href, window.location.href);
          var authorID = parsed.searchParams.get("author_id") || parsed.searchParams.get("authorid");
          if (authorID) {
            return authorID;
          }
        }
      }
    } catch (error) {
      return "";
    }
    return "";
  }

  function queryValue(name) {
    try {
      return new URL(window.location.href).searchParams.get(name) || "";
    } catch (error) {
      return "";
    }
  }

  function pageMetadata(cgiData, cgiDataNew) {
    return {
      nickname: firstValue(
        window.nickname,
        objectValue(cgiData, "nick_name"),
        objectValue(cgiDataNew, "nick_name"),
        elementText([
          "#js_name",
          "#profile_nickname",
          "#js_profile_nickname",
          ".rich_media_meta_nickname",
          ".profile_nickname",
          "[data-nickname]"
        ]),
        elementAttribute(["meta[property='og:site_name']", "meta[name='author']"], ["content"])
      ),
      avatar_url: firstValue(
        window.headimg,
        objectValue(cgiData, "round_head_img"),
        objectValue(cgiData, "hd_head_img"),
        objectValue(cgiDataNew, "round_head_img"),
        objectValue(cgiDataNew, "hd_head_img"),
        elementAttribute([
          "#js_profile_avatar",
          "#profile_avatar",
          ".profile_avatar img",
          ".profile_inner img",
          ".account_avatar img",
          "[data-avatar]"
        ], ["src", "data-src", "data-original", "data-avatar"])
      ),
      author_id: firstValue(
        window.author_id,
        window.authorId,
        objectValue(cgiData, "author_id"),
        objectValue(cgiData, "user_name"),
        objectValue(cgiDataNew, "author_id"),
        objectValue(cgiDataNew, "user_name"),
        queryValue("author_id"),
        queryValue("authorid"),
        authorIDFromLink()
      )
    };
  }

  function credentials() {
    var cgiData = window.cgiData || {};
    var cgiDataNew = window.cgiDataNew || {};
    var metadata = pageMetadata(cgiData, cgiDataNew);
    var pageLink = firstValue(objectValue(cgiDataNew, "link"), objectValue(cgiData, "link"));
    var biz = firstValue(
      config.biz,
      window.biz,
      window.__biz,
      objectValue(cgiData, "biz"),
      objectValue(cgiData, "bizuin"),
      objectValue(cgiDataNew, "biz"),
      objectValue(cgiDataNew, "bizuin"),
      queryValue("__biz"),
      queryValue("biz"),
      urlValue(pageLink, "__biz"),
      urlValue(pageLink, "biz")
    );
    return {
      biz: biz,
      nickname: metadata.nickname,
      avatar_url: metadata.avatar_url,
      author_id: metadata.author_id,
      uin: firstValue(window.uin, objectValue(cgiData, "uin"), objectValue(cgiData, "user_uin"), objectValue(cgiDataNew, "uin"), objectValue(cgiDataNew, "user_uin"), queryValue("uin")),
      key: firstValue(window.key, objectValue(cgiData, "key"), objectValue(cgiDataNew, "key"), queryValue("key")),
      pass_ticket: firstValue(window.pass_ticket, objectValue(cgiData, "pass_ticket"), objectValue(cgiDataNew, "pass_ticket"), queryValue("pass_ticket")),
      appmsg_token: firstValue(window.appmsg_token, objectValue(cgiData, "appmsg_token"), objectValue(cgiDataNew, "appmsg_token"), queryValue("appmsg_token")),
      refresh_uri: buildRefreshURI(cgiData, cgiDataNew, biz)
    };
  }

  function buildRefreshURI(cgiData, cgiDataNew, biz) {
    var pageLink = firstValue(objectValue(cgiDataNew, "link"), objectValue(cgiData, "link"));
    var linkBiz = firstValue(biz, urlValue(window.location.href, "__biz"), urlValue(pageLink, "__biz"));
    var mid = firstValue(objectValue(cgiDataNew, "mid"), urlValue(window.location.href, "mid"), urlValue(pageLink, "mid"));
    var idx = firstValue(objectValue(cgiDataNew, "idx"), urlValue(window.location.href, "idx"), urlValue(pageLink, "idx"), "1");
    var sn = firstValue(objectValue(cgiDataNew, "sn"), urlValue(window.location.href, "sn"), urlValue(pageLink, "sn"));
    if (linkBiz && mid && sn) {
      return "https://mp.weixin.qq.com/s?__biz=" + encodeURIComponent(linkBiz) +
        "&mid=" + encodeURIComponent(mid) +
        "&idx=" + encodeURIComponent(idx) +
        "&sn=" + encodeURIComponent(sn);
    }
    return firstValue(pageLink, window.location.href);
  }

  function refreshURL(endpoint) {
    var origin = String(config.origin || "").replace(/\/+$/, "");
    var path = (origin ? origin : "") + (endpoint || "/api/mp/refresh");
    if (config.token) {
      path += (path.indexOf("?") >= 0 ? "&" : "?") + "token=" + encodeURIComponent(config.token);
    }
    return path;
  }

  function buildArticleCommentURL(account) {
    if (!account || !account.biz || !account.key || !isArticlePage()) {
      return "";
    }
    var cgiDataNew = window.cgiDataNew || {};
    var cgiData = window.cgiData || {};
    var appmsgid = firstValue(
      objectValue(cgiDataNew, "appmsgid"),
      objectValue(cgiDataNew, "appmsg_id"),
      objectValue(cgiDataNew, "mid"),
      objectValue(cgiData, "appmsgid"),
      objectValue(cgiData, "appmsg_id"),
      objectValue(cgiData, "mid"),
      queryValue("appmsgid"),
      queryValue("mid")
    );
    var idx = firstValue(
      objectValue(cgiDataNew, "idx"),
      objectValue(cgiData, "idx"),
      queryValue("idx"),
      "1"
    );
    var commentID = firstValue(
      objectValue(cgiDataNew, "comment_id"),
      objectValue(cgiDataNew, "commentId"),
      objectValue(cgiDataNew, "segment_comment_id"),
      objectValue(cgiDataNew, "segmentCommentId"),
      objectValue(cgiDataNew, "extra_comment_id"),
      objectValue(cgiDataNew, "extraCommentId"),
      objectValue(cgiData, "comment_id"),
      objectValue(cgiData, "commentId"),
      queryValue("comment_id")
    );
    if (!appmsgid || !commentID) {
      return "";
    }
    var origin = firstValue(window.location && window.location.origin, "https://mp.weixin.qq.com");
    var target;
    try {
      target = new URL("/mp/appmsg_comment", origin);
    } catch (error) {
      return "";
    }
    var query = target.searchParams;
    query.set("action", "getcomment");
    query.set("__biz", account.biz);
    query.set("scene", "0");
    query.set("uin", account.uin);
    query.set("key", account.key);
    query.set("pass_ticket", account.pass_ticket);
    query.set("wxtoken", "");
    query.set("x5", "0");
    query.set("f", "json");
    query.set("devicetype", "UnifiedPCWindows");
    query.set("clientversion", "f2541c37");
    query.set("appmsgid", appmsgid);
    query.set("idx", idx);
    query.set("comment_id", commentID);
    query.set("offset", "0");
    query.set("limit", "100");
    if (account.appmsg_token) {
      query.set("appmsg_token", account.appmsg_token);
    }
    return target.toString();
  }

  function requestArticleComments(account) {
    if (articleCommentRequestStarted) {
      return false;
    }
    var target = buildArticleCommentURL(account);
    if (!target) {
      return false;
    }
    articleCommentRequestStarted = true;
    fetch(target, {
      method: "GET",
      headers: { Accept: "application/json, text/plain, */*", "X-Requested-With": "XMLHttpRequest" },
      credentials: "include"
    }).then(function (response) {
      if (!response || response.ok === false) {
        throw new Error("comment request failed");
      }
      if (typeof response.clone === "function") {
        var copy = response.clone();
        if (copy && typeof copy.text === "function") {
          return copy.text().then(function (body) {
            inspectNetworkValue(body, 0);
          });
        }
      }
      return null;
    }).catch(function () {
      // The backend keeps the article-page fallback; a failed page request
      // must not break the existing passive network observer.
    });
    return true;
  }

  function isOfficialAccountPage() {
    var path = window.location.pathname || "";
    return path === "/s" || path.indexOf("/s/") === 0 ||
      path === "/mp/profile_ext" || path === "/mp/author" || path === "/mp/getappmsgext";
  }

  function isArticlePage() {
    var path = window.location.pathname || "";
    return path === "/s" || path.indexOf("/s/") === 0;
  }

  function apiOrigin() {
    return String(config.origin || "http://127.0.0.1:2026").replace(/\/+$/, "");
  }

  function buildRSSURL(account) {
    if (!account || !account.biz) {
      return "";
    }
    return apiOrigin() + "/rss/mp?biz=" + encodeURIComponent(account.biz) + "&proxy=1";
  }

  function buildConsoleURL() {
    var configured = firstValue(config.console_origin, config.consoleOrigin, config.web_origin, config.webOrigin);
    if (configured) {
      return configured.replace(/\/+$/, "") + "/console";
    }

    try {
      var origin = new URL(apiOrigin());
      var port = Number(origin.port || 0);
      origin.port = port > 1 ? String(port - 1) : "2025";
      origin.pathname = "/console";
      origin.search = "";
      origin.hash = "";
      return origin.toString().replace(/\/+$/, "");
    } catch (error) {
      return "http://127.0.0.1:2025/console";
    }
  }

  function copyWithSelection(value) {
    return new Promise(function (resolve, reject) {
      var textarea = document.createElement("textarea");
      textarea.value = value;
      textarea.setAttribute("readonly", "readonly");
      textarea.style.position = "fixed";
      textarea.style.top = "0";
      textarea.style.left = "-9999px";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      var copied = false;
      try {
        if (typeof textarea.focus === "function") {
          textarea.focus();
        }
        if (typeof textarea.select === "function") {
          textarea.select();
        }
        if (typeof textarea.setSelectionRange === "function") {
          textarea.setSelectionRange(0, value.length);
        }
        if (typeof document.execCommand === "function") {
          copied = document.execCommand("copy") !== false;
        }
      } catch (error) {
        // Keep the rejection so the panel can show a manual-copy prompt.
      } finally {
        if (textarea.parentNode) {
          textarea.parentNode.removeChild(textarea);
        } else if (document.body && typeof document.body.removeChild === "function") {
          document.body.removeChild(textarea);
        }
      }
      if (copied) {
        resolve();
      } else {
        reject(new Error("clipboard unavailable"));
      }
    });
  }

  function copyText(value) {
    var writeText = navigator && navigator.clipboard && navigator.clipboard.writeText;
    if (typeof writeText === "function") {
      try {
        return Promise.resolve(writeText.call(navigator, value)).catch(function () {
          return copyWithSelection(value);
        });
      } catch (error) {
        return copyWithSelection(value);
      }
    }
    return copyWithSelection(value);
  }

  function showNotice(message, isError) {
    if (!document.body || !message) {
      return;
    }
    var notice = document.getElementById(noticeId);
    if (!notice) {
      notice = document.createElement("div");
      notice.id = noticeId;
      notice.style.cssText = "position:fixed;top:16px;left:50%;transform:translateX(-50%);z-index:2147483647;max-width:70vw;padding:8px 14px;border-radius:6px;background:rgba(31,41,55,.94);color:#fff;font:13px/1.45 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;box-shadow:0 4px 16px rgba(0,0,0,.2);pointer-events:none;";
      document.body.appendChild(notice);
    }
    notice.textContent = message;
    notice.style.background = isError ? "rgba(180,35,24,.94)" : "rgba(31,41,55,.94)";
    if (notice.__wx_channels_notice_timer__ && typeof window.clearTimeout === "function") {
      window.clearTimeout(notice.__wx_channels_notice_timer__);
    }
    if (typeof window.setTimeout === "function") {
      notice.__wx_channels_notice_timer__ = window.setTimeout(function () {
        if (notice.parentNode && typeof notice.parentNode.removeChild === "function") {
          notice.parentNode.removeChild(notice);
        }
      }, 2600);
    }
  }

  function copyRSSAddress(account) {
    var current = account || credentials();
    var rss = buildRSSURL(current);
    if (!rss) {
      showNotice("未找到公众号信息，请保持文章页面打开后重试", true);
      return Promise.reject(new Error("official account biz is missing"));
    }
    return copyText(rss).then(function () {
      showNotice("RSS 地址已复制", false);
      setStatus("RSS 地址已复制", false);
      return rss;
    }).catch(function (error) {
      showNotice("复制失败，请手动复制 RSS 地址", true);
      setStatus("复制失败，请手动复制", true);
      throw error;
    });
  }

  function readJSONResponse(response) {
    if (!response) {
      return Promise.reject(new Error("没有收到服务器响应"));
    }
    var read;
    try {
      if (typeof response.json === "function") {
        read = response.json();
      } else if (typeof response.text === "function") {
        read = response.text().then(function (body) {
          return body ? JSON.parse(body) : {};
        });
      } else {
        return Promise.reject(new Error("服务器响应不可读取"));
      }
    } catch (error) {
      return Promise.reject(error);
    }
    return Promise.resolve(read).then(function (payload) {
      if (response.ok === false) {
        throw new Error((payload && (payload.message || payload.error)) || ("HTTP " + (response.status || "unknown")));
      }
      if (payload && payload.code !== undefined && payload.code !== 0) {
        throw new Error(payload.message || "接口返回失败");
      }
      return payload;
    });
  }

  function articleItemsFromPayload(payload) {
    var data = payload && payload.data !== undefined ? payload.data : payload;
    data = data && typeof data === "object" ? data : {};
    var result = [];

    function append(item, fallbackTime) {
      if (!item || typeof item !== "object") {
        return;
      }
      var candidate = item.app_msg_ext_info || item.appMsgExtInfo || item;
      if (!candidate || typeof candidate !== "object") {
        return;
      }
      var publishTime = firstValue(candidate.publish_time, candidate.publishTime, item.publish_time, item.publishTime, fallbackTime);
        var article = {
          title: firstValue(candidate.title, item.title),
          url: firstValue(candidate.url, candidate.content_url, candidate.contentUrl, candidate.source_url, candidate.sourceUrl, item.url),
          video_id: firstValue(candidate.video_id, candidate.videoId, candidate.vid, candidate.mpvid, item.video_id, item.videoId),
          digest: firstValue(candidate.digest, item.digest),
          publish_time: publishTime,
          subtype: firstValue(candidate.subtype, item.subtype),
          copyright_stat: firstValue(candidate.copyright_stat, candidate.copyrightStat, item.copyright_stat),
          duration: firstValue(candidate.duration, candidate.duration_seconds, candidate.durationSeconds, candidate.video_duration, candidate.videoDuration, item.duration),
          audio_fileid: firstValue(candidate.audio_fileid, candidate.audioFileID, candidate.audioFileId, item.audio_fileid),
          play_url: firstValue(candidate.play_url, candidate.playUrl, candidate.video_url, candidate.videoUrl, item.play_url),
          item_show_type: firstValue(candidate.item_show_type, candidate.itemShowType, item.item_show_type)
      };
      if (article.title || article.url) {
        result.push(article);
      }
      var children = candidate.multi_app_msg_item_list || candidate.multiAppMsgItemList;
      if (Array.isArray(children)) {
        for (var i = 0; i < children.length; i += 1) {
          append(children[i], publishTime);
        }
      }
    }

    if (Array.isArray(data.articles)) {
      for (var i = 0; i < data.articles.length; i += 1) {
        append(data.articles[i], "");
      }
    }
    if (!result.length && Array.isArray(data.list)) {
      for (var j = 0; j < data.list.length; j += 1) {
        append(data.list[j], objectValue(data.list[j] && data.list[j].comm_msg_info, "datetime"));
      }
    }

    var seen = {};
    return result.filter(function (item) {
      var key = item.url || item.title + "\u0001" + item.publish_time;
      if (!key || seen[key]) {
        return false;
      }
      seen[key] = true;
      return true;
    });
  }

  function formatArticleTime(value) {
    var timestamp = Number(value || 0);
    if (timestamp > 100000000000) {
      timestamp = Math.floor(timestamp / 1000);
    }
    if (!timestamp) {
      return "";
    }
    var date = new Date(timestamp * 1000);
    if (isNaN(date.getTime())) {
      return "";
    }
    function pad(number) {
      return number < 10 ? "0" + number : String(number);
    }
    return date.getFullYear() + "-" + pad(date.getMonth() + 1) + "-" + pad(date.getDate()) + " " +
      pad(date.getHours()) + ":" + pad(date.getMinutes());
  }

  function closeMessageList() {
    var dialog = document.getElementById(messageListDialogId);
    if (dialog && dialog.parentNode && typeof dialog.parentNode.removeChild === "function") {
      dialog.parentNode.removeChild(dialog);
    }
  }

  function renderMessageList(articles, account) {
    if (!document.body) {
      return;
    }
    closeMessageList();
    var overlay = document.createElement("div");
    overlay.id = messageListDialogId;
    overlay.style.cssText = "position:fixed;inset:0;z-index:2147483646;display:flex;align-items:center;justify-content:center;padding:20px;background:rgba(15,23,42,.38);backdrop-filter:blur(3px);-webkit-backdrop-filter:blur(3px);font:13px/1.45 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;color:#1f2937;";
    overlay.addEventListener("click", function (event) {
      if (!event || event.target === overlay) {
        closeMessageList();
      }
    });

    var dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.setAttribute("aria-label", "推送列表");
    dialog.style.cssText = "width:92vw;max-width:720px;max-height:82vh;display:flex;flex-direction:column;overflow:hidden;border:1px solid rgba(16,24,40,.10);border-radius:12px;background:#fff;box-shadow:0 24px 72px rgba(15,23,42,.24),0 4px 16px rgba(15,23,42,.10);";
    dialog.addEventListener("click", function (event) {
      if (event && typeof event.stopPropagation === "function") {
        event.stopPropagation();
      }
    });
    overlay.appendChild(dialog);

    var header = document.createElement("div");
    header.style.cssText = "display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 18px 13px;border-bottom:1px solid #eaecf0;background:#fff;";
    var heading = document.createElement("div");
    heading.textContent = (account && account.nickname ? account.nickname + " - " : "") + "推送列表 (" + articles.length + ")";
    heading.style.cssText = "min-width:0;overflow:hidden;color:#101828;font-weight:600;font-size:15px;white-space:nowrap;text-overflow:ellipsis;";
    header.appendChild(heading);
    var closeButton = document.createElement("button");
    closeButton.type = "button";
    closeButton.setAttribute("aria-label", "关闭");
    closeButton.setAttribute("title", "关闭");
    closeButton.style.cssText = "display:inline-flex;align-items:center;justify-content:center;flex:0 0 32px;width:32px;height:32px;padding:0;border:0;border-radius:8px;background:transparent;color:#667085;cursor:pointer;transition:background .16s ease,color .16s ease,transform .16s ease;";
    closeButton.appendChild(createArticleIcon("close", "wx-channels-mp-close-icon"));
    closeButton.addEventListener("mouseenter", function () {
      closeButton.style.background = "#f2f4f7";
      closeButton.style.color = "#344054";
    });
    closeButton.addEventListener("mouseleave", function () {
      closeButton.style.background = "transparent";
      closeButton.style.color = "#667085";
    });
    closeButton.addEventListener("focus", function () {
      closeButton.style.boxShadow = "0 0 0 3px rgba(7,193,96,.16)";
    });
    closeButton.addEventListener("blur", function () {
      closeButton.style.boxShadow = "none";
    });
    closeButton.addEventListener("click", closeMessageList);
    header.appendChild(closeButton);
    dialog.appendChild(header);

    var list = document.createElement("div");
    list.style.cssText = "overflow:auto;padding:8px 18px 18px;overscroll-behavior:contain;";
    if (!articles.length) {
      var empty = document.createElement("div");
      empty.textContent = "当前没有可显示的推送文章";
      empty.style.cssText = "padding:32px 0;text-align:center;color:#667085;";
      list.appendChild(empty);
    }
    function bindMessageRowHover(row) {
      row.addEventListener("mouseenter", function () {
        row.style.background = "#f6fef9";
      });
      row.addEventListener("mouseleave", function () {
        row.style.background = "transparent";
      });
    }
    function bindMessageLinkHover(link) {
      link.addEventListener("mouseenter", function () {
        link.style.color = "#067647";
      });
      link.addEventListener("mouseleave", function () {
        link.style.color = "#175cd3";
      });
    }
    for (var i = 0; i < articles.length; i += 1) {
      var item = articles[i];
      var row = document.createElement("div");
      row.style.cssText = "display:flex;gap:12px;align-items:flex-start;margin:0 -10px;padding:11px 10px;border-bottom:1px solid #f2f4f7;border-radius:8px;transition:background .16s ease;";
      bindMessageRowHover(row);
      var index = document.createElement("span");
      index.textContent = String(i + 1);
      index.style.cssText = "flex:0 0 24px;padding-top:1px;color:#98a2b3;text-align:right;font-variant-numeric:tabular-nums;";
      row.appendChild(index);
      var content = document.createElement("div");
      content.style.cssText = "min-width:0;flex:1;";
      if (item.url) {
        var link = document.createElement("a");
        link.href = item.url;
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.textContent = item.title || item.url;
        link.style.cssText = "display:block;color:#175cd3;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;transition:color .16s ease;";
        bindMessageLinkHover(link);
        content.appendChild(link);
      } else {
        var title = document.createElement("div");
        title.textContent = item.title || "未命名文章";
        title.style.cssText = "white-space:nowrap;overflow:hidden;text-overflow:ellipsis;";
        content.appendChild(title);
      }
      if (item.digest) {
        var digest = document.createElement("div");
        digest.textContent = item.digest;
        digest.style.cssText = "margin-top:3px;color:#667085;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;";
        content.appendChild(digest);
      }
      var time = formatArticleTime(item.publish_time);
      if (time) {
        var timeLabel = document.createElement("div");
        timeLabel.textContent = time;
        timeLabel.style.cssText = "margin-top:3px;color:#98a2b3;font-size:12px;";
        content.appendChild(timeLabel);
      }
      row.appendChild(content);
      list.appendChild(row);
    }
    dialog.appendChild(list);
    document.body.appendChild(overlay);
  }

  function loadMessageList() {
    var account = credentials();
    if (!account.biz) {
      showNotice("未找到公众号信息，请保持文章页面打开后重试", true);
      return Promise.reject(new Error("official account biz is missing"));
    }
    showNotice("正在读取推送列表...", false);
    var endpoint = "/api/mp/msg/list?biz=" + encodeURIComponent(account.biz);
    return fetch(refreshURL(endpoint), {
      method: "GET",
      headers: { Accept: "application/json, text/plain, */*" },
      credentials: "omit"
    }).then(readJSONResponse).then(function (payload) {
      var articles = articleItemsFromPayload(payload);
      renderMessageList(articles, account);
      showNotice("已读取 " + articles.length + " 篇推送", false);
      return articles;
    }).catch(function (error) {
      showNotice("推送列表读取失败: " + error.message, true);
      throw error;
    });
  }

  function articleHTML() {
    var cgiDataNew = window.cgiDataNew || {};
    var cgiData = window.cgiData || {};
    var content = firstValue(
      objectValue(cgiDataNew, "content_noencode"),
      objectValue(cgiData, "content_noencode")
    );
    if (content) {
      return content;
    }
    try {
      var article = document.querySelector("#js_content");
      return article && article.innerHTML ? String(article.innerHTML) : "";
    } catch (error) {
      return "";
    }
  }

  function articleArchiveHTML() {
    try {
      var article = document.querySelector("#js_content");
      if (article) {
        if (article.outerHTML) {
          return String(article.outerHTML);
        }
        if (article.innerHTML) {
          return '<div id="js_content">' + String(article.innerHTML) + '</div>';
        }
      }
    } catch (error) {
      // Fall back to the page data object below.
    }
    var content = articleHTML();
    return content ? '<div id="js_content">' + content + '</div>' : "";
  }

  function numericArticleValue() {
    for (var i = 0; i < arguments.length; i += 1) {
      var value = arguments[i];
      if (value === undefined || value === null || String(value).trim() === "") {
        continue;
      }
      var parsed = Number(String(value).replace(/,/g, "").trim());
      if (isFinite(parsed)) {
        return Math.round(parsed);
      }
    }
    return 0;
  }

  function firstObjectValue() {
    for (var i = 0; i < arguments.length; i += 1) {
      if (arguments[i] && typeof arguments[i] === "object") {
        return arguments[i];
      }
    }
    return {};
  }

  function firstMediaObject() {
    for (var i = 0; i < arguments.length; i += 1) {
      var value = arguments[i];
      if (Array.isArray(value)) {
        for (var j = 0; j < value.length; j += 1) {
          if (value[j] && typeof value[j] === "object" && !Array.isArray(value[j])) {
            return value[j];
          }
        }
      } else if (value && typeof value === "object") {
        return value;
      }
    }
    return {};
  }

  function firstArrayValue() {
    for (var i = 0; i < arguments.length; i += 1) {
      if (Array.isArray(arguments[i]) && arguments[i].length) {
        return arguments[i];
      }
    }
    return [];
  }

  function firstMediaValue(objects, keys) {
    for (var i = 0; i < objects.length; i += 1) {
      var object = objects[i];
      if (!object || typeof object !== "object" || Array.isArray(object)) {
        continue;
      }
      for (var j = 0; j < keys.length; j += 1) {
        var value = objectValue(object, keys[j]);
        if (value !== undefined && value !== null && String(value).trim() !== "") {
          return value;
        }
      }
    }
    return "";
  }

  function firstPositiveArticleValue() {
    for (var i = 0; i < arguments.length; i += 1) {
      var parsed = numericArticleValue(arguments[i]);
      if (parsed > 0) {
        return parsed;
      }
    }
    return 0;
  }

  function articleDOMElements(selectors) {
    var result = [];
    if (typeof document === "undefined" || !document || typeof document.querySelectorAll !== "function") {
      return result;
    }
    for (var i = 0; i < selectors.length; i += 1) {
      try {
        var elements = document.querySelectorAll(selectors[i]) || [];
        for (var j = 0; j < elements.length; j += 1) {
          if (result.indexOf(elements[j]) < 0) {
            result.push(elements[j]);
          }
        }
      } catch (error) {
        // Keep trying selectors supported by the current WebView.
      }
    }
    return result;
  }

  function articleDOMAttribute(element, names) {
    if (!element) {
      return "";
    }
    for (var i = 0; i < names.length; i += 1) {
      var name = names[i];
      try {
        if (typeof element.getAttribute === "function") {
          var attribute = element.getAttribute(name);
          if (attribute !== undefined && attribute !== null && String(attribute).trim() !== "") {
            return String(attribute).trim();
          }
        }
      } catch (error) {
        // Continue with the remaining attributes and properties.
      }
      try {
        var property = element[name];
        if (property !== undefined && property !== null && String(property).trim() !== "") {
          return String(property).trim();
        }
      } catch (error) {
        // Some WebView media elements throw while reading a property.
      }
    }
    return "";
  }

  function articleDOMQueryValue(rawURL, names) {
    if (!rawURL) {
      return "";
    }
    try {
      var parsed = new URL(rawURL, window.location.href);
      for (var i = 0; i < names.length; i += 1) {
        var value = parsed.searchParams.get(names[i]);
        if (value) {
          return value;
        }
      }
    } catch (error) {
      // The source may be a relative or malformed embed URL.
    }
    return "";
  }

  function articleDOMDurationSeconds(element) {
    var milliseconds = articleDOMAttribute(element, ["data-duration-ms", "duration_ms", "durationMs"]);
    var millisecondsValue = numericArticleValue(milliseconds);
    if (millisecondsValue > 0) {
      return Math.round(millisecondsValue / 1000);
    }

    var raw = articleDOMAttribute(element, ["data-duration", "duration", "video-duration", "videoDuration"]);
    if (raw) {
      var parts = String(raw).trim().split(":");
      if (parts.length === 2 || parts.length === 3) {
        var seconds = Number(parts.pop());
        var minutes = Number(parts.pop());
        var hours = parts.length ? Number(parts.pop()) : 0;
        if (isFinite(hours) && isFinite(minutes) && isFinite(seconds) &&
          hours >= 0 && minutes >= 0 && seconds >= 0) {
          return Math.round(hours * 3600 + minutes * 60 + seconds);
        }
      }
      var parsed = numericArticleValue(raw);
      if (parsed > 0) {
        return parsed;
      }
    }

    try {
      var duration = Number(element && element.duration);
      if (isFinite(duration) && duration > 0) {
        return Math.round(duration);
      }
    } catch (error) {
      // Media metadata may not be available until the element is loaded.
    }
    return 0;
  }

  function articleDOMMediaObjects() {
    var result = [];
    var elements = articleDOMElements([
      "#js_content video",
      "#js_content iframe.video_iframe",
      "#js_content mp-common-mpaudio",
      "#js_content [data-mpvid]",
      "#js_content [data-vid]",
      "#js_content [data-video-id]",
      "#js_content [voice_encode_fileid]",
      "video",
      "iframe.video_iframe",
      "mp-common-mpaudio",
      "[data-mpvid]",
      "[data-vid]",
      "[data-video-id]",
      "[voice_encode_fileid]"
    ]);
    for (var i = 0; i < elements.length; i += 1) {
      var element = elements[i];
      var tagName = String(element && (element.tagName || element.nodeName) || "").toLowerCase();
      var videoID = articleDOMAttribute(element, [
        "data-vid",
        "vid",
        "data-mpvid",
        "mpvid",
        "data-video-id",
        "video-id",
        "data-media-id",
        "mediaid"
      ]);
      var embedURL = articleDOMAttribute(element, ["src", "data-src", "data-url"]);
      if (!videoID) {
        videoID = articleDOMQueryValue(embedURL, ["vid", "video_id", "videoid", "mpvid", "mediaid"]);
      }
      var playURL = "";
      if (tagName !== "iframe") {
        playURL = articleDOMAttribute(element, [
          "data-play-url",
          "data-video-url",
          "data-media-url",
          "data-url",
          "data-src",
          "src",
          "currentSrc"
        ]);
      } else {
        playURL = articleDOMAttribute(element, ["data-play-url", "data-video-url", "data-media-url"]);
      }
      var audioFileID = articleDOMAttribute(element, [
        "voice_encode_fileid",
        "data-audio-fileid",
        "audio_fileid",
        "audioFileID"
      ]);
      var media = {
        video_id: videoID,
        duration: articleDOMDurationSeconds(element),
        audio_fileid: audioFileID,
        play_url: playURL,
        cover_url: articleDOMAttribute(element, ["poster", "data-cover", "cover"])
      };
      if (media.video_id || media.duration || media.audio_fileid || media.play_url) {
        result.push(media);
      }
    }
    return result;
  }

  function articleMediaMetadata(cgiDataNew, cgiData) {
    var newVideoPageInfo = firstMediaObject(
      objectValue(cgiDataNew, "video_page_info"),
      objectValue(cgiData, "video_page_info"),
      objectValue(cgiDataNew, "videoPageInfo"),
      objectValue(cgiData, "videoPageInfo"),
      pageVideoPageInfoObjects,
      window.video_page_info,
      window.videoPageInfo
    );
    var transfer = firstMediaObject(
      firstArrayValue(
        objectValue(newVideoPageInfo, "mp_video_trans_info"),
        objectValue(newVideoPageInfo, "mpVideoTransInfo"),
        objectValue(cgiDataNew, "mp_video_trans_info"),
        objectValue(cgiData, "mp_video_trans_info")
      )[0],
      objectValue(cgiDataNew, "mpVideoTransInfo"),
      objectValue(cgiData, "mpVideoTransInfo"),
      pageVideoTransferObjects,
      window.mp_video_trans_info,
      window.mpVideoTransInfo
    );
    var videoPages = firstArrayValue(
      objectValue(cgiDataNew, "video_page_infos"),
      objectValue(cgiData, "video_page_infos"),
      objectValue(cgiDataNew, "videoPageInfos"),
      objectValue(cgiData, "videoPageInfos"),
      pageVideoPageInfos,
      window.videoPageInfos,
      window.video_page_infos
    );
    var domMediaObjects = articleDOMMediaObjects();
    var firstVideo = firstMediaObject(videoPages[0], domMediaObjects[0]);
    if (!Object.keys(transfer).length) {
      transfer = firstMediaObject(
        firstArrayValue(
          objectValue(firstVideo, "mp_video_trans_info"),
          objectValue(firstVideo, "mpVideoTransInfo")
        )[0],
        objectValue(firstVideo, "video_info"),
        objectValue(firstVideo, "videoInfo")
      );
    }
    var copyrightInfo = firstObjectValue(objectValue(cgiDataNew, "copyright_info"), objectValue(cgiData, "copyright_info"));
    var mediaObjects = [
      transfer,
      firstVideo,
      newVideoPageInfo,
      firstMediaObject(objectValue(newVideoPageInfo, "video_info"), objectValue(newVideoPageInfo, "videoInfo"))
    ].concat(pageMediaObjects, domMediaObjects);
    var rawDuration = firstMediaValue(mediaObjects, [
      "duration",
      "duration_seconds",
      "durationSeconds",
      "video_duration",
      "videoDuration",
      "video_play_len",
      "videoPlayLen"
    ]);
    var duration = firstPositiveArticleValue(
      rawDuration,
      objectValue(cgiDataNew, "duration"),
      objectValue(cgiData, "duration"),
      objectValue(cgiDataNew, "video_duration"),
      objectValue(cgiData, "video_duration")
    );
    if (!duration) {
      var durationMs = firstPositiveArticleValue(
        objectValue(cgiDataNew, "duration_ms"),
        objectValue(cgiData, "duration_ms"),
        objectValue(cgiDataNew, "video_duration_ms"),
        objectValue(cgiData, "video_duration_ms"),
        firstMediaValue(mediaObjects, ["duration_ms", "durationMs", "video_duration_ms", "videoDurationMs"])
      );
      if (durationMs > 0) {
        duration = Math.round(durationMs / 1000);
      }
    }
    return {
      video_id: firstValue(
        objectValue(cgiDataNew, "video_id"),
        objectValue(cgiDataNew, "videoId"),
        objectValue(cgiDataNew, "vid"),
        objectValue(cgiDataNew, "mpvid"),
        objectValue(cgiData, "video_id"),
        objectValue(cgiData, "videoId"),
        objectValue(cgiData, "vid"),
        objectValue(cgiData, "mpvid"),
        firstMediaValue(mediaObjects, ["video_id", "videoId", "vid", "mpvid", "media_id", "mediaId"])
      ),
      subtype: firstPositiveArticleValue(
        objectValue(cgiDataNew, "subtype"),
        objectValue(cgiData, "subtype"),
        firstMediaValue(mediaObjects, ["subtype"])
      ),
      copyright_stat: firstPositiveArticleValue(
        objectValue(cgiDataNew, "copyright_stat"),
        objectValue(cgiData, "copyright_stat"),
        objectValue(copyrightInfo, "copyright_stat"),
        firstMediaValue(mediaObjects, ["copyright_stat", "copyrightStat"])
      ),
      duration: duration,
      audio_fileid: firstPositiveArticleValue(
        objectValue(cgiDataNew, "audio_fileid"),
        objectValue(cgiData, "audio_fileid"),
        firstMediaValue(mediaObjects, ["audio_fileid", "audioFileID", "audioFileId", "voice_encode_fileid", "voiceEncodeFileID"])
      ),
      play_url: firstValue(
        objectValue(cgiDataNew, "play_url"),
        objectValue(cgiDataNew, "playUrl"),
        objectValue(cgiDataNew, "video_url"),
        objectValue(cgiDataNew, "videoUrl"),
        objectValue(cgiData, "play_url"),
        objectValue(cgiData, "playUrl"),
        objectValue(cgiData, "video_url"),
        objectValue(cgiData, "videoUrl"),
        firstMediaValue(mediaObjects, ["play_url", "playUrl", "video_url", "videoUrl", "media_url", "mediaUrl", "url"])
      ),
      item_show_type: firstPositiveArticleValue(
        objectValue(cgiDataNew, "item_show_type"),
        objectValue(cgiData, "item_show_type"),
        firstMediaValue(mediaObjects, ["item_show_type", "itemShowType"])
      ),
      malicious_title_reason_id: firstPositiveArticleValue(
        objectValue(cgiDataNew, "malicious_title_reason_id"),
        objectValue(cgiData, "malicious_title_reason_id"),
        firstMediaValue(mediaObjects, ["malicious_title_reason_id", "maliciousTitleReasonID"])
      ),
      malicious_content_type: firstPositiveArticleValue(
        objectValue(cgiDataNew, "malicious_content_type"),
        objectValue(cgiData, "malicious_content_type"),
        firstMediaValue(mediaObjects, ["malicious_content_type", "maliciousContentType"])
      )
    };
  }

  function articleMetadata(account) {
    var cgiDataNew = window.cgiDataNew || {};
    var cgiData = window.cgiData || {};
    var media = articleMediaMetadata(cgiDataNew, cgiData);
    var contentURL = firstValue(
      window.msg_link,
      window.msgLink,
      objectValue(cgiDataNew, "link"),
      objectValue(cgiData, "link"),
      window.location && window.location.href
    );
    var publishTime = firstValue(
      window.ct,
      window.create_time,
      window.publish_time,
      objectValue(cgiDataNew, "publish_time"),
      objectValue(cgiData, "publish_time")
    );
    var numericPublishTime = Number(publishTime || 0);
    if (!isFinite(numericPublishTime)) {
      numericPublishTime = 0;
    }
    return {
      title: firstValue(
        window.msg_title,
        window.msgTitle,
        objectValue(cgiDataNew, "title"),
        objectValue(cgiData, "title"),
        elementText(["#activity-name", "#js_title", ".rich_media_title", "h1"]),
        elementAttribute(["meta[property='og:title']", "meta[name='twitter:title']"], ["content"])
      ),
      digest: firstValue(
        window.msg_digest,
        window.msgDigest,
        objectValue(cgiDataNew, "digest"),
        objectValue(cgiData, "digest"),
        elementAttribute(["meta[name='description']", "meta[property='og:description']"], ["content"])
      ),
      content_url: contentURL,
      source_url: firstValue(
        window.source_url,
        objectValue(cgiDataNew, "source_url"),
        objectValue(cgiData, "source_url"),
        contentURL
      ),
      cover: firstValue(
        window.msg_cdn_url,
        window.msgCdnUrl,
        objectValue(cgiDataNew, "cover"),
        objectValue(cgiDataNew, "cover_url"),
        objectValue(cgiData, "cover"),
        objectValue(cgiData, "cover_url"),
        elementAttribute(["meta[property='og:image']", "meta[name='twitter:image']"], ["content"])
      ),
      author: firstValue(
        window.msg_author,
        window.author,
        objectValue(cgiDataNew, "author"),
        objectValue(cgiData, "author"),
        elementAttribute(["meta[name='author']"], ["content"]),
        account && account.nickname
      ),
      publish_time: numericPublishTime,
      fileid: numericArticleValue(objectValue(cgiDataNew, "fileid"), objectValue(cgiData, "fileid")),
      video_id: media.video_id,
      mid: firstValue(objectValue(cgiDataNew, "mid"), objectValue(cgiData, "mid"), queryValue("mid")),
      idx: numericArticleValue(objectValue(cgiDataNew, "idx"), objectValue(cgiData, "idx"), queryValue("idx"), 1),
      is_multi: numericArticleValue(objectValue(cgiDataNew, "is_multi"), objectValue(cgiData, "is_multi")),
      is_original: numericArticleValue(objectValue(cgiDataNew, "is_original"), objectValue(cgiData, "is_original")),
      is_paid: numericArticleValue(objectValue(cgiDataNew, "is_paid"), objectValue(cgiData, "is_paid")),
      is_pay_subscribe: numericArticleValue(objectValue(cgiDataNew, "is_pay_subscribe"), objectValue(cgiData, "is_pay_subscribe")),
      subtype: media.subtype,
      copyright_stat: media.copyright_stat,
      duration: media.duration,
      audio_fileid: media.audio_fileid,
      play_url: media.play_url,
      item_show_type: media.item_show_type,
      malicious_title_reason_id: media.malicious_title_reason_id,
      malicious_content_type: media.malicious_content_type
    };
  }

  function metricValueFromText(text, labels) {
    var source = String(text || "").trim();
    if (!source) {
      return null;
    }
    var lower = source.toLowerCase();
    for (var i = 0; i < labels.length; i += 1) {
      var index = lower.indexOf(String(labels[i]).toLowerCase());
      if (index >= 0) {
        var value = metricNumber(source.slice(index + String(labels[i]).length));
        if (value !== null) {
          return value;
        }
      }
    }
    return metricNumber(source);
  }

  function metricValueFromSelectors(name, selectors) {
    var text = elementText(selectors);
    var value = metricValueFromText(text, metricLabels[name] || []);
    if (value !== null) {
      return value;
    }
    var attribute = elementAttribute(selectors, ["data-count", "data-value", "aria-label", "title", "value"]);
    return metricValueFromText(attribute, metricLabels[name] || []);
  }

  function pageMetrics() {
    inspectNetworkValue(window.cgiData, 0);
    inspectNetworkValue(window.cgiDataNew, 0);
    var values = {};
    Object.keys(pageMetricData).forEach(function (name) {
      values[name] = pageMetricData[name];
    });
    var windowFields = {
      view_count: ["read_num3", "readNum3", "read_num", "readNum", "view_count", "viewCount", "reading_count", "readingCount", "read_count", "readCount"],
      like_count: ["like_num", "likeNum", "like_count", "likeCount", "old_like_num", "oldLikeNum", "old_like_count", "praise_count", "praiseCount"],
      comment_count: ["elected_comment_total_cnt", "electedCommentTotalCnt", "comment_total_cnt", "commentTotalCnt", "comment_count", "commentCount", "comment_num", "commentNum", "cmt_count", "cmtCount"],
      share_count: ["share_count", "shareCount", "share_num", "shareNum", "forward_count", "forwardCount", "repost_count", "repostCount"],
      collect_count: ["collect_count", "collectCount", "collect_num", "collectNum", "favorite_count", "favoriteCount", "fav_count", "favCount"],
      reward_count: ["reward_count", "rewardCount", "reward_num", "rewardNum", "appmsg_reward_count", "appmsgRewardCount", "tip_count", "tipCount"]
    };
    Object.keys(windowFields).forEach(function (name) {
      if (values[name] !== undefined) {
        return;
      }
      for (var i = 0; i < windowFields[name].length; i += 1) {
        var value = metricNumber(window[windowFields[name][i]]);
        if (value !== null) {
          values[name] = value;
          break;
        }
      }
    });
    var selectors = {
      view_count: ["#readNum3", "#readNum", "#js_read_area", ".read_num", ".readNum", "[data-read-count]"],
      like_count: ["#likeNum", "#js_like_btn", ".like_num", ".likeNum", "[data-like-count]"],
      comment_count: ["#commentCount", "#js_comment_count", ".comment_count", ".commentCount", "[data-comment-count]"],
      share_count: ["#shareCount", ".share_count", ".shareCount", "[data-share-count]", "[data-forward-count]"],
      collect_count: ["#collectCount", ".collect_count", ".collectCount", "[data-collect-count]", "[data-favorite-count]"],
      reward_count: ["#rewardCount", ".reward_count", ".rewardCount", "[data-reward-count]"]
    };
    Object.keys(selectors).forEach(function (name) {
      if (values[name] === undefined) {
        var value = metricValueFromSelectors(name, selectors[name]);
        if (value !== null) {
          values[name] = value;
        }
      }
    });
    return values;
  }

  function articleMetricsHTML() {
    try {
      var root = document.documentElement;
      var markup = root && root.outerHTML ? String(root.outerHTML) : "";
      if (markup.length > 8 * 1024 * 1024) {
        return markup.slice(0, 8 * 1024 * 1024);
      }
      return markup;
    } catch (error) {
      return "";
    }
  }

  function articleMetricFingerprint(account, article, metrics) {
    return [account.biz, article.content_url, article.source_url, JSON.stringify(metrics)].join("\u0001");
  }

  function submitArticleMetrics(account) {
    if (!isArticlePage() || !account || !account.biz) {
      return false;
    }
    var article = articleMetadata(account);
    if (!article.content_url) {
      return false;
    }
    var metrics = pageMetrics();
    var html = articleMetricsHTML();
    if (!Object.keys(metrics).length && !html) {
      return false;
    }
    var fingerprint = articleMetricFingerprint(account, article, metrics);
    if (metricSubmitted[fingerprint]) {
      return true;
    }
    metricSubmitted[fingerprint] = true;
    var request = {
      biz: account.biz,
      article: article,
      metrics: metrics,
      source: Object.keys(pageMetricData).length ? "network" : "article_page"
    };
    if (html) {
      request.html = html;
    }
    fetch(refreshURL("/api/mp/metrics"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(request),
      credentials: "omit"
    }).then(readJSONResponse).then(function (payload) {
      var result = payload && payload.data ? payload.data : {};
      if (result.stored) {
        setStatus("文章指标已记录", false);
      }
    }).catch(function (error) {
      delete metricSubmitted[fingerprint];
      setStatus("文章指标采集失败: " + error.message, true);
    });
    return true;
  }

  function downloadArticle() {
    if (articleDownloadPromise) {
      return articleDownloadPromise;
    }
    var account = credentials();
    if (!account.biz) {
      showNotice("未找到公众号信息，请保持文章页面打开后重试", true);
      return Promise.reject(new Error("official account biz is missing"));
    }
    var content = articleArchiveHTML();
    if (!content) {
      showNotice("文章内容尚未加载，请稍后重试", true);
      return Promise.reject(new Error("article HTML is empty"));
    }
    var request = {
      biz: account.biz,
      article: articleMetadata(account),
      html: content
    };
    showNotice("正在下载文章及图片...", false);
    articleDownloadPromise = fetch(refreshURL("/api/mp/archive/download"), {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(request),
      credentials: "omit"
    }).then(readJSONResponse).then(function (payload) {
      var result = payload && payload.data ? payload.data : {};
      var failed = Number(result.failed || 0);
      var message = failed > 0
        ? "文章已保存，但有 " + failed + " 张图片下载失败"
        : "文章下载完成";
      showNotice(message, failed > 0);
      setStatus(message, failed > 0);
      articleDownloadPromise = null;
      return result;
    }, function (error) {
      articleDownloadPromise = null;
      showNotice("文章下载失败: " + error.message, true);
      setStatus("文章下载失败: " + error.message, true);
      throw error;
    });
    return articleDownloadPromise;
  }

  function copyArticleHTML() {
    var content = articleHTML();
    if (!content) {
      showNotice("文章 HTML 尚未加载，请稍后重试", true);
      return Promise.reject(new Error("article HTML is empty"));
    }
    return copyText(content).then(function () {
      showNotice("文章 HTML 已复制", false);
      return content;
    }).catch(function (error) {
      showNotice("文章 HTML 复制失败，请手动复制", true);
      throw error;
    });
  }

  function openConsole() {
    var url = buildConsoleURL();
    try {
      if (typeof window.open === "function") {
        var opened = window.open(url, "_blank");
        if (opened === null) {
          showNotice("管理控制台被浏览器拦截，请允许打开新窗口", true);
        } else {
          showNotice("已打开管理控制台", false);
        }
        return url;
      }
    } catch (error) {
      showNotice("管理控制台打开失败: " + error.message, true);
      return "";
    }
    showNotice("当前浏览器不支持打开管理控制台", true);
    return "";
  }

  function hideToolsMenu(root) {
    var menu = root && root.__wx_channels_menu__;
    if (menu) {
      menu.style.display = "none";
    }
    var trigger = root && root.__wx_channels_menu_trigger__;
    if (trigger && typeof trigger.setAttribute === "function") {
      trigger.setAttribute("aria-expanded", "false");
    }
  }

  function positionToolsMenu(root, trigger, menu) {
    if (!trigger || typeof trigger.getBoundingClientRect !== "function") {
      return;
    }
    var rect = trigger.getBoundingClientRect();
    var width = 236;
    var height = 232;
    var viewportWidth = Number(window.innerWidth || 0);
    var viewportHeight = Number(window.innerHeight || 0);
    var left = Math.max(8, rect.right - width);
    var top = rect.bottom + 8;
    if (viewportWidth > 0) {
      left = Math.min(left, Math.max(8, viewportWidth - width - 8));
    }
    if (viewportHeight > 0 && top + height > viewportHeight && rect.top > height + 8) {
      top = rect.top - height - 8;
    }
    menu.style.left = Math.round(left) + "px";
    menu.style.top = Math.round(top) + "px";
  }

  function createArticleIcon(name, className) {
    var icon = document.createElement("span");
    icon.className = className || "wx-channels-mp-icon";
    icon.setAttribute("aria-hidden", "true");
    icon.style.cssText = "display:inline-flex;align-items:center;justify-content:center;flex:0 0 18px;width:18px;height:18px;pointer-events:none;";
    icon.innerHTML = articleMenuIcons[name] || "";
    return icon;
  }

  function menuItem(label, action, root, iconName) {
    var item = document.createElement("button");
    item.type = "button";
    item.setAttribute("role", "menuitem");
    item.setAttribute("aria-label", label);
    item.style.cssText = "display:flex;align-items:center;gap:10px;width:100%;min-height:42px;padding:0 12px;border:0;border-radius:7px;background:transparent;color:#344054;cursor:pointer;text-align:left;font:14px/1.35 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;white-space:nowrap;transition:background .16s ease,color .16s ease,transform .16s ease,box-shadow .16s ease;";
    item.appendChild(createArticleIcon(iconName));
    var text = document.createElement("span");
    text.textContent = label;
    text.style.cssText = "min-width:0;overflow:hidden;text-overflow:ellipsis;";
    item.appendChild(text);
    var hovered = false;
    var focused = false;
    function refreshItemState() {
      var active = hovered || focused;
      item.style.background = active ? "#ecfdf3" : "transparent";
      item.style.color = active ? "#067647" : "#344054";
      item.style.transform = active ? "translateX(1px)" : "translateX(0)";
      item.style.boxShadow = focused ? "inset 3px 0 0 #07c160" : "none";
    }
    item.addEventListener("mouseenter", function () {
      hovered = true;
      refreshItemState();
    });
    item.addEventListener("mouseleave", function () {
      hovered = false;
      refreshItemState();
    });
    item.addEventListener("focus", function () {
      focused = true;
      refreshItemState();
    });
    item.addEventListener("blur", function () {
      focused = false;
      refreshItemState();
    });
    item.addEventListener("keydown", function (event) {
      if (event && event.key === "Escape") {
        hideToolsMenu(root);
      }
    });
    item.addEventListener("click", function (event) {
      if (event && typeof event.preventDefault === "function") {
        event.preventDefault();
      }
      if (event && typeof event.stopPropagation === "function") {
        event.stopPropagation();
      }
      hideToolsMenu(root);
      try {
        var result = action();
        if (result && typeof result.catch === "function") {
          result.catch(function () {
            // The action already reports the contextual error to the user.
          });
        }
      } catch (error) {
        showNotice(label + "失败: " + error.message, true);
      }
    });
    return item;
  }

  function createArticleTools() {
    var root = document.createElement("div");
    root.id = toolsRootId;
    root.className = "sns_opr_btn_con wx-channels-mp-tools-root";
    root.style.cssText = "position:relative;display:inline-flex;align-items:center;overflow:visible;";

    var trigger = document.createElement("button");
    trigger.type = "button";
    trigger.className = "wx-channels-mp-tools-trigger";
    trigger.setAttribute("aria-haspopup", "menu");
    trigger.setAttribute("aria-expanded", "false");
    trigger.setAttribute("aria-label", "下载");
    trigger.setAttribute("title", "文章工具");
    trigger.style.cssText = "display:inline-flex;align-items:center;gap:7px;min-height:38px;padding:0 10px;border:0;border-radius:8px;background:transparent;color:#667085;cursor:pointer;font:15px/1 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;transition:background .16s ease,color .16s ease,box-shadow .16s ease,transform .16s ease;";
    trigger.appendChild(createArticleIcon("download", "wx-channels-mp-tools-icon"));
    var triggerText = document.createElement("span");
    triggerText.textContent = "下载";
    trigger.appendChild(triggerText);
    root.appendChild(trigger);

    var menu = document.createElement("div");
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-hidden", "true");
    menu.style.cssText = "position:fixed;display:none;width:236px;max-height:calc(100vh - 24px);overflow:auto;padding:6px;border:1px solid rgba(16,24,40,.10);border-radius:10px;background:rgba(255,255,255,.98);box-shadow:0 16px 36px rgba(16,24,40,.16),0 2px 8px rgba(16,24,40,.08);backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);font:14px/1.35 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;";
    root.appendChild(menu);
    root.__wx_channels_menu__ = menu;
    root.__wx_channels_menu_trigger__ = trigger;

    menu.appendChild(menuItem("复制文章HTML", copyArticleHTML, root, "copy"));
    menu.appendChild(menuItem("复制RSS地址", function () {
      return copyRSSAddress(credentials());
    }, root, "rss"));
    menu.appendChild(menuItem("下载文章", downloadArticle, root, "archive"));
    menu.appendChild(menuItem("推送列表", loadMessageList, root, "list"));
    menu.appendChild(menuItem("管理控制台", openConsole, root, "console"));

    function setTriggerState(active) {
      trigger.style.background = active ? "rgba(7,193,96,.10)" : "transparent";
      trigger.style.color = active ? "#067647" : "#667085";
      trigger.style.transform = active ? "translateY(-1px)" : "translateY(0)";
    }

    function showMenu(event) {
      if (event && typeof event.stopPropagation === "function") {
        event.stopPropagation();
      }
      menu.style.display = "block";
      menu.setAttribute("aria-hidden", "false");
      positionToolsMenu(root, trigger, menu);
      setTriggerState(true);
      trigger.setAttribute("aria-expanded", "true");
    }
    var hideTimer = 0;
    function cancelHideTimer() {
      if (hideTimer && typeof window.clearTimeout === "function") {
        window.clearTimeout(hideTimer);
        hideTimer = 0;
      }
    }
    function hideMenu(event) {
      if (event && typeof event.stopPropagation === "function") {
        event.stopPropagation();
      }
      menu.style.display = "none";
      menu.setAttribute("aria-hidden", "true");
      trigger.setAttribute("aria-expanded", "false");
      setTriggerState(false);
    }
    root.addEventListener("mouseenter", function () {
      cancelHideTimer();
      showMenu();
    });
    root.addEventListener("mouseleave", function () {
      cancelHideTimer();
      if (typeof window.setTimeout === "function") {
        hideTimer = window.setTimeout(hideMenu, 160);
      } else {
        hideMenu();
      }
    });
    menu.addEventListener("mouseenter", cancelHideTimer);
    trigger.addEventListener("mouseenter", function () {
      setTriggerState(true);
    });
    trigger.addEventListener("mouseleave", function () {
      if (menu.style.display !== "block") {
        setTriggerState(false);
      }
    });
    trigger.addEventListener("focus", function () {
      setTriggerState(true);
      trigger.style.boxShadow = "0 0 0 3px rgba(7,193,96,.16)";
    });
    trigger.addEventListener("blur", function () {
      trigger.style.boxShadow = "none";
      if (menu.style.display !== "block") {
        setTriggerState(false);
      }
    });
    trigger.addEventListener("keydown", function (event) {
      if (!event) {
        return;
      }
      if (event.key === "Escape") {
        hideMenu(event);
        return;
      }
      if (event.key === "ArrowDown") {
        if (typeof event.preventDefault === "function") {
          event.preventDefault();
        }
        showMenu(event);
        var firstItem = menu.children && menu.children[0];
        if (firstItem && typeof firstItem.focus === "function") {
          firstItem.focus();
        }
      }
    });
    trigger.addEventListener("click", function (event) {
      if (menu.style.display === "block") {
        hideMenu(event);
      } else {
        showMenu(event);
      }
    });
    menu.addEventListener("click", function (event) {
      if (event && typeof event.stopPropagation === "function") {
        event.stopPropagation();
      }
    });
    document.addEventListener("click", hideMenu);
    root.__wx_channels_show_menu__ = showMenu;
    root.__wx_channels_hide_menu__ = hideMenu;
    return root;
  }

  function mountArticleTools() {
    if (!isArticlePage() || !document.body) {
      return !isArticlePage();
    }
    var existing = document.getElementById(toolsRootId);
    if (existing) {
      return true;
    }
    var wraps = document.querySelectorAll(".interaction_bar");
    if (!wraps || !wraps.length) {
      return false;
    }
    var container = wraps[wraps.length - 1];
    if (!container || typeof container.appendChild !== "function") {
      return false;
    }
    var root = createArticleTools();
    var last = container.lastElementChild;
    if (last && typeof container.insertBefore === "function") {
      container.insertBefore(root, last);
    } else {
      container.appendChild(root);
    }
    return true;
  }

  function setStatus(message, isError) {
    var panel = document.getElementById(panelId);
    if (!panel) {
      return;
    }
    var status = panel.querySelector("[data-status]");
    if (status) {
      status.textContent = message;
      status.style.color = isError ? "#b42318" : "#18794e";
    }
  }

  function renderPanel(account, allowArticleFallback) {
    if (!document.body || document.getElementById(panelId) || !account || !account.biz || (isArticlePage() && !allowArticleFallback)) {
      return;
    }
    var panel = document.createElement("div");
    panel.id = panelId;
    panel.style.cssText = "position:fixed;top:14px;right:14px;z-index:2147483647;width:220px;padding:12px;border:1px solid rgba(0,0,0,.12);border-radius:8px;background:rgba(255,255,255,.96);box-shadow:0 6px 24px rgba(0,0,0,.16);font:13px/1.45 -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;color:#1f2937;";

    var title = document.createElement("div");
    title.setAttribute("data-account-title", "1");
    title.textContent = account.nickname || "公众号文章采集";
    title.style.cssText = "font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;margin-bottom:4px;";
    panel.appendChild(title);

    var status = document.createElement("div");
    status.setAttribute("data-status", "1");
    status.textContent = "正在读取页面凭证...";
    status.style.cssText = "color:#667085;margin-bottom:9px;";
    panel.appendChild(status);

    var button = document.createElement("button");
    button.type = "button";
    button.textContent = "复制 RSS 地址";
    button.style.cssText = "width:100%;padding:7px 9px;border:0;border-radius:6px;background:#1f6feb;color:white;cursor:pointer;font:inherit;";
    button.addEventListener("click", function () {
      var current = panel.__wx_channels_account__ || account;
      copyRSSAddress(current).catch(function () {
        // The panel status already contains the contextual error.
      });
    });
    panel.appendChild(button);
    panel.__wx_channels_account__ = account;
    document.body.appendChild(panel);
  }

  function updatePanel(account) {
    if (!account) {
      return;
    }
    var panel = document.getElementById(panelId);
    if (!panel) {
      renderPanel(account);
      panel = document.getElementById(panelId);
    }
    if (!panel) {
      return;
    }
    panel.__wx_channels_account__ = account;
    var title = panel.querySelector("[data-account-title]") || panel.children[0];
    if (title && account.nickname) {
      title.textContent = account.nickname;
    }
  }

  function accountFingerprint(account, hasCredential) {
    return [
      account.biz,
      hasCredential ? account.key : "",
      account.nickname,
      account.avatar_url,
      account.author_id,
      account.uin,
      account.pass_ticket,
      account.appmsg_token
    ].join("\u0001");
  }

  function submitAccount(account) {
    var hasCredential = !!account.key;
    var hasMetadata = !!(account.nickname || account.avatar_url || account.author_id);
    if (!account.biz || (!hasCredential && !hasMetadata)) {
      return false;
    }
    var endpoint = hasCredential ? "/api/mp/refresh" : "/api/mp/metadata";
    var dedupeKey = accountFingerprint(account, hasCredential);
    if (submitted[dedupeKey]) {
      return true;
    }
    submitted[dedupeKey] = true;
    fetch(refreshURL(endpoint), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(account),
      credentials: "omit"
    }).then(function (response) {
      if (!response.ok) {
        throw new Error("HTTP " + response.status);
      }
      setStatus(
        hasCredential && account.author_id
          ? "公众号凭证已采集"
          : hasCredential
            ? "公众号凭证已采集，正在补全账户信息..."
            : "公众号账户信息已同步",
        false
      );
    }).catch(function (error) {
      delete submitted[dedupeKey];
      setStatus("采集失败: " + error.message, true);
    });
    return true;
  }

  function start() {
    if (!isOfficialAccountPage()) {
      return;
    }
    installNetworkObservers();
    var account = credentials();
    var articlePage = isArticlePage();
    var articleToolsMounted = mountArticleTools();
    renderPanel(account);
    updatePanel(account);
    var attempts = 0;
    var timer = window.setInterval(function () {
      attempts += 1;
      account = credentials();
      if (articlePage && !articleToolsMounted) {
        articleToolsMounted = mountArticleTools();
      }
      updatePanel(account);
      if (account.biz && (account.key || account.nickname || account.avatar_url || account.author_id)) {
        submitAccount(account);
      }
      if (articlePage) {
        requestArticleComments(account);
        submitArticleMetrics(account);
      }
      var hasMetadata = !!(account.nickname || account.avatar_url || account.author_id);
      if (account.biz && account.author_id && (account.key || hasMetadata) && (!articlePage || articleToolsMounted)) {
		if (articlePage) {
			setStatus("公众号账户信息已采集", false);
			return;
		}
        window.clearInterval(timer);
        setStatus("公众号账户信息已采集", false);
        return;
      }
      if (attempts >= maxCaptureAttempts) {
        window.clearInterval(timer);
        if (articlePage && !articleToolsMounted) {
          renderPanel(account, true);
        }
        setStatus("未找到完整公众号账户信息，请保持公众号页面打开后刷新", true);
      }
    }, 500);
  }

  function onReady() {
    window.setTimeout(start, 600);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", onReady);
  } else {
    onReady();
  }
})();
