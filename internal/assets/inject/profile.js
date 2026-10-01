/**
 * @file Profile页面功能模块 - 事件监听和数据采集
 */

function __wx_is_profile_page__() {
  return window.location.pathname.includes('/pages/profile');
}

function __wx_is_account_like_page__() {
  return window.location.pathname.includes('/pages/account/like');
}

function __wx_is_profile_like_list_page__() {
  return __wx_is_profile_page__() || __wx_is_account_like_page__();
}

function __wx_profile_list_page_title__() {
  return __wx_is_account_like_page__() ? '赞和收藏 - 视频列表' : 'Profile - 视频列表';
}

function __wx_get_like_label_by_key__(key) {
  if (key === 'fav') return '收藏';
  if (key === 'like') return '点赞';
  if (key === 'global_fav') return '看一看';
  return '全部';
}

function __wx_get_account_like_subtab_info__() {
  if (!__wx_is_account_like_page__()) {
    return { key: 'default', label: '全部' };
  }

  var activeTab = document.querySelector('.sub-tab-item.active');
  if (!activeTab) {
    return { key: 'all', label: '全部' };
  }

  var text = (activeTab.textContent || '').trim();
  if (text) {
    return { key: text, label: text };
  }

  var iconUse = activeTab.querySelector('use');
  var href = iconUse ? (iconUse.getAttribute('xlink:href') || iconUse.getAttribute('href') || '') : '';
  if (href.indexOf('icon-account_fav') !== -1) {
    return { key: 'fav', label: '收藏' };
  }
  if (href.indexOf('icon-account_like') !== -1) {
    return { key: 'like', label: '点赞' };
  }
  if (href.indexOf('icon-account_global_fav') !== -1) {
    return { key: 'global_fav', label: '看一看' };
  }

  return { key: 'all', label: '全部' };
}

function __wx_normalize_compare_text__(text) {
  return (text || '')
    .replace(/\s+/g, '')
    .replace(/[#@].*$/g, '')
    .trim();
}

// ==================== Profile页面视频列表采集器 ====================
window.__wx_channels_profile_collector = {
  videos: [],
  likeTabVideos: {},
  currentLikeTabKey: 'all',
  currentLikeTabLabel: '全部',
  likeTabFlags: {
    all: 7,
  },
  isCollecting: false,
  _lastLogMessage: '',
  _lastTipVideoCount: 0,
  _lastTipLiveReplayCount: 0,
  _maxVideos: 100000, // 最多采集100000个视频

  // 初始化
  init: function () {
    var self = this;
    this.syncCurrentLikeTab();
    // 延迟初始化UI
    setTimeout(function () {
      self.injectToolbarDownloadIcon();
    }, 2000);
    if (__wx_is_account_like_page__()) {
      this.startLikeTabMonitor();
      this.installLikeApiHook();
    }
  },

  syncCurrentLikeTab: function () {
    if (!__wx_is_account_like_page__()) return;
    var info = __wx_get_account_like_subtab_info__();
    this.currentLikeTabKey = info.key;
    this.currentLikeTabLabel = info.label;
    if (!this.likeTabVideos[this.currentLikeTabKey]) {
      this.likeTabVideos[this.currentLikeTabKey] = [];
    }
    this.videos = this.likeTabVideos[this.currentLikeTabKey];
  },

  startLikeTabMonitor: function () {
    var self = this;
    if (window.__wx_account_like_tab_monitor_started__) return;
    window.__wx_account_like_tab_monitor_started__ = true;

    document.addEventListener('click', function (event) {
      var target = event.target;
      if (!target) return;
      var tab = target.closest ? target.closest('.sub-tab-item') : null;
      if (!tab) return;
      if (tab.id === 'wx-profile-download-btn') return;
      setTimeout(function () {
        self.syncCurrentLikeTab();
        if (window.__wx_batch_download_manager__ && window.__wx_batch_download_manager__.isVisible) {
          if (self.videos.length > 0) {
            var filteredVideos = self.filterLivePictureVideos(self.videos).filter(function (v) {
              return v && (v.type === 'media' || v.type === 'live_replay');
            });
            __update_batch_download_ui__(filteredVideos, '赞和收藏 - ' + self.currentLikeTabLabel);
          } else {
            __close_batch_download_ui__();
          }
        }
      }, 80);
    }, true);
  },

  resolveLikeTabKeyByFlag: function (flag) {
    var normalized = String(flag);
    var keys = Object.keys(this.likeTabFlags);
    for (var i = 0; i < keys.length; i++) {
      if (String(this.likeTabFlags[keys[i]]) === normalized) {
        return keys[i];
      }
    }
    return this.currentLikeTabKey || 'all';
  },

  installLikeApiHook: function () {
    if (!__wx_is_account_like_page__()) return;
    if (window.__wx_like_api_hook_installed__) return;

    var self = this;
    var tryInstall = function () {
      if (!WXU.API4 || typeof WXU.API4.finderGetInteractionedFeedList !== 'function') {
        return false;
      }
      if (WXU.API4.finderGetInteractionedFeedList.__wx_wrapped__) {
        window.__wx_like_api_hook_installed__ = true;
        return true;
      }

      var original = WXU.API4.finderGetInteractionedFeedList;
      var wrapped = async function (payload) {
        var response = await original.apply(this, arguments);
        try {
          var feeds = response && response.data && Array.isArray(response.data.object) ? response.data.object : [];
          var tabFlag = payload && payload.tabFlag ? payload.tabFlag : null;
          if (tabFlag != null) {
            var mappedKey = self.resolveLikeTabKeyByFlag(tabFlag);
            self.likeTabFlags[mappedKey] = tabFlag;
          }
          WXE.emit(WXE.Events.InteractionedFeedsLoaded, {
            feeds: feeds,
            tabFlag: tabFlag,
          });
        } catch (e) {
          console.warn('[Profile] 处理 interactioned list 响应失败:', e);
        }
        return response;
      };
      wrapped.__wx_wrapped__ = true;
      wrapped.__wx_original__ = original;
      WXU.API4.finderGetInteractionedFeedList = wrapped;
      window.__wx_like_api_hook_installed__ = true;
      console.log('[Profile] ✅ 已安装 account/like API4 响应监听');
      return true;
    };

    if (tryInstall()) return;

    WXE.onAPILoaded(function () {
      tryInstall();
    });
  },

  getCurrentLikeTabDOMItems: function () {
    if (!__wx_is_account_like_page__()) return [];
    return Array.prototype.slice.call(document.querySelectorAll('.card-grid .card-wrp'));
  },

  getCurrentLikeTabFirstCardTitle: function () {
    var cards = this.getCurrentLikeTabDOMItems();
    if (!cards.length) return '';
    var titleNode = cards[0].querySelector('.title');
    if (!titleNode) return '';
    var fullTitle = titleNode.getAttribute('title') || titleNode.textContent || '';
    return this.cleanHtmlTags(fullTitle);
  },

  inferLikeTabFlagFromDOM: async function () {
    if (!__wx_is_account_like_page__()) return null;
    if (this.likeTabFlags[this.currentLikeTabKey]) {
      return this.likeTabFlags[this.currentLikeTabKey];
    }

    var firstTitle = this.getCurrentLikeTabFirstCardTitle();
    var normalizedFirstTitle = __wx_normalize_compare_text__(firstTitle);
    var candidateFlags = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
    for (var i = 0; i < candidateFlags.length; i++) {
      var flag = candidateFlags[i];
      try {
        var result = await WXU.API4.finderGetInteractionedFeedList({ lastBuffer: '', tabFlag: flag });
        var items = result && result.data && Array.isArray(result.data.object) ? result.data.object : [];
        if (!items.length) continue;

        if (!normalizedFirstTitle) {
          this.likeTabFlags[this.currentLikeTabKey] = flag;
          return flag;
        }

        var firstProfile = WXU.format_feed(items[0]);
        var fetchedTitle = this.cleanHtmlTags(firstProfile && firstProfile.title ? firstProfile.title : '');
        var normalizedFetchedTitle = __wx_normalize_compare_text__(fetchedTitle);
        var shortCurrent = normalizedFirstTitle.slice(0, 12);
        var shortFetched = normalizedFetchedTitle.slice(0, 12);

        if (
          normalizedFetchedTitle &&
          (
            normalizedFetchedTitle === normalizedFirstTitle ||
            normalizedFetchedTitle.indexOf(shortCurrent) !== -1 ||
            normalizedFirstTitle.indexOf(shortFetched) !== -1
          )
        ) {
          this.likeTabFlags[this.currentLikeTabKey] = flag;
          return flag;
        }
      } catch (e) {
        console.warn('[Profile] 推断 account/like tabFlag 失败:', flag, e);
      }
    }

    return null;
  },

  loadCurrentLikeTabVideos: async function () {
    if (!__wx_is_account_like_page__()) return [];
    this.syncCurrentLikeTab();

    if (this.likeTabVideos[this.currentLikeTabKey] && this.likeTabVideos[this.currentLikeTabKey].length > 0) {
      this.videos = this.likeTabVideos[this.currentLikeTabKey];
      return this.videos;
    }

    if (!WXU.API4 || typeof WXU.API4.finderGetInteractionedFeedList !== 'function') {
      throw new Error('页面 API4 尚未初始化');
    }

    var flag = await this.inferLikeTabFlagFromDOM();
    if (!flag) {
      throw new Error('未能识别当前标签的数据类型');
    }

    var nextMarker = '';
    var hasMore = true;
    var merged = [];
    var seen = {};

    while (hasMore) {
      var response = await WXU.API4.finderGetInteractionedFeedList({
        lastBuffer: nextMarker,
        tabFlag: flag,
      });

      if (!response || response.errCode) {
        throw new Error((response && response.errMsg) || '拉取当前标签数据失败');
      }

      var items = response && response.data && Array.isArray(response.data.object) ? response.data.object : [];
      for (var i = 0; i < items.length; i++) {
        var profile = WXU.format_feed(items[i]);
        if (!profile || !profile.id || seen[profile.id]) continue;
        seen[profile.id] = true;
        merged.push(profile);
      }

      nextMarker = response && response.data ? (response.data.lastBuffer || '') : '';
      hasMore = !!nextMarker && items.length > 0;
      if (items.length < 15) {
        hasMore = false;
      }
    }

    this.likeTabVideos[this.currentLikeTabKey] = merged;
    this.videos = merged;
    return merged;
  },

  // 在Profile页面操作区注入批量下载按钮
  injectToolbarDownloadIcon: function () {
    var self = this;

    var findActionContainer = function () {
      if (__wx_is_profile_page__()) {
        return document.querySelector('.profile-info .opr-area') ||
          document.querySelector('.opr-area.mb-6.mt-6') ||
          document.querySelector('[class*="profile-info"] [class*="opr-area"]');
      }
      if (__wx_is_account_like_page__()) {
        return document.querySelector('[data-v-3a10b0ca].flex.flex-initial.flex-shrink-0.items-center.space-x-3.pb-5') ||
          document.querySelector('.flex.flex-initial.flex-shrink-0.items-center.space-x-3.pb-5');
      }
      return null;
    };

    var tryInject = function () {
      var container = findActionContainer();
      if (!container) return false;
      if (container.querySelector('#wx-profile-download-btn')) return true;

      var button = document.createElement('button');
      button.id = 'wx-profile-download-btn';
      button.type = 'button';
      if (__wx_is_account_like_page__()) {
        button.className = 'wx-like-download-btn flex cursor-pointer items-center justify-center border-0 border-b-2 border-solid pb-0.5 pt-[5px] text-sm';
        button.style.marginLeft = '8px';
        button.style.background = 'transparent';
        button.style.color = 'inherit';
        button.style.borderColor = 'transparent';
        button.style.flexShrink = '0';
        button.style.opacity = '0.88';
        button.title = '批量下载当前列表视频';
        button.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" class="mx-1 !h-4 !w-4"><path fill-rule="evenodd" clip-rule="evenodd" d="M12 3C12.3314 3 12.6 3.26863 12.6 3.6V13.1515L15.5757 10.1757C15.8101 9.94142 16.1899 9.94142 16.4243 10.1757C16.6586 10.4101 16.6586 10.7899 16.4243 11.0243L12.4243 15.0243C12.1899 15.2586 11.8101 15.2586 11.5757 15.0243L7.57574 11.0243C7.34142 10.7899 7.34142 10.4101 7.57574 10.1757C7.81005 9.94142 8.18995 9.94142 8.42426 10.1757L11.4 13.1515V3.6C11.4 3.26863 11.6686 3 12 3ZM3.6 14.4C3.93137 14.4 4.2 14.6686 4.2 15V19.2C4.2 19.5314 4.46863 19.8 4.8 19.8H19.2C19.5314 19.8 19.8 19.5314 19.8 19.2V15C19.8 14.6686 20.0686 14.4 20.4 14.4C20.7314 14.4 21 14.6686 21 15V19.2C21 20.1941 20.1941 21 19.2 21H4.8C3.80589 21 3 20.1941 3 19.2V15C3 14.6686 3.26863 14.4 3.6 14.4Z" fill="currentColor"></path></svg><div>批量下载</div>';
        button.onmouseenter = function () {
          button.style.opacity = '1';
        };
        button.onmouseleave = function () {
          button.style.opacity = '0.88';
        };
      } else {
        button.className = 'weui-btn_default relative flex h-7 flex-shrink-0 cursor-pointer items-center justify-center rounded-md text-sm';
        button.style.width = '96px';
        button.style.marginLeft = '8px';
        button.title = '批量下载当前账号视频';
        button.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" class="h-4 w-4 flex-shrink-0 text-fg-0"><path fill-rule="evenodd" clip-rule="evenodd" d="M12 3C12.3314 3 12.6 3.26863 12.6 3.6V13.1515L15.5757 10.1757C15.8101 9.94142 16.1899 9.94142 16.4243 10.1757C16.6586 10.4101 16.6586 10.7899 16.4243 11.0243L12.4243 15.0243C12.1899 15.2586 11.8101 15.2586 11.5757 15.0243L7.57574 11.0243C7.34142 10.7899 7.34142 10.4101 7.57574 10.1757C7.81005 9.94142 8.18995 9.94142 8.42426 10.1757L11.4 13.1515V3.6C11.4 3.26863 11.6686 3 12 3ZM3.6 14.4C3.93137 14.4 4.2 14.6686 4.2 15V19.2C4.2 19.5314 4.46863 19.8 4.8 19.8H19.2C19.5314 19.8 19.8 19.5314 19.8 19.2V15C19.8 14.6686 20.0686 14.4 20.4 14.4C20.7314 14.4 21 14.6686 21 15V19.2C21 20.1941 20.1941 21 19.2 21H4.8C3.80589 21 3 20.1941 3 19.2V15C3 14.6686 3.26863 14.4 3.6 14.4Z" fill="currentColor"></path></svg><div class="ml-1 min-w-0 flex-shrink-0 whitespace-nowrap text-fg-0">批量下载</div>';
      }

      // 点击事件 - 显示/隐藏批量下载面板
      button.onclick = async function () {
        // 使用通用批量下载组件
        if (window.__wx_batch_download_manager__ && window.__wx_batch_download_manager__.isVisible) {
          __close_batch_download_ui__();
        } else {
          if (__wx_is_account_like_page__()) {
            try {
              __wx_log({ msg: '⏳ 正在加载「' + self.currentLikeTabLabel + '」数据...' });
              await self.loadCurrentLikeTabVideos();
            } catch (e) {
              __wx_log({ msg: '❌ ' + (e.message || e) });
              return;
            }
          }

          // 显示批量下载UI（包含视频和直播回放，排除正在直播）
          var filteredVideos = self.filterLivePictureVideos(self.videos).filter(function (v) {
            return v && (v.type === 'media' || v.type === 'live_replay');
          });

          if (filteredVideos.length === 0) {
            __wx_log({ msg: '⚠️ 暂无视频数据' });
            return;
          }

          var title = __wx_is_account_like_page__()
            ? ('赞和收藏 - ' + self.currentLikeTabLabel)
            : __wx_profile_list_page_title__();
          __show_batch_download_ui__(filteredVideos, title);
        }
      };

      if (__wx_is_account_like_page__()) {
        container.appendChild(button);
      } else {
        var shopWrapper = container.querySelector('.shop-btn__wrp');
        if (shopWrapper && shopWrapper.parentNode === container) {
          container.insertBefore(button, shopWrapper);
        } else {
          container.appendChild(button);
        }
      }

      console.log('[Profile] ✅ 批量下载按钮已注入到操作区');
      return true;
    };

    if (tryInject()) return;

    var observer = new MutationObserver(function (mutations, obs) {
      if (tryInject()) { obs.disconnect(); }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    setTimeout(function () { observer.disconnect(); }, 5000);
  },

  // 过滤掉正在直播的图片类型数据
  filterLivePictureVideos: function (videos) {
    return (videos || []).filter(function (v) {
      if (v.type === 'picture' && v.contact && v.contact.liveStatus === 1) {
        return false;
      }
      return true;
    });
  },

  // 清理HTML标签
  cleanHtmlTags: function (text) {
    if (!text || typeof text !== 'string') return text || '';
    var tempDiv = document.createElement('div');
    tempDiv.innerHTML = text;
    var cleaned = tempDiv.textContent || tempDiv.innerText || '';
    return cleaned.trim();
  },

  // 从API添加单个视频
  addVideoFromAPI: function (videoData) {
    if (!videoData || !videoData.id) return;

    // 过滤掉正在直播的图片类型数据
    if (videoData.type === 'picture' && videoData.contact && videoData.contact.liveStatus === 1) {
      return;
    }

    if (__wx_is_account_like_page__()) {
      this.syncCurrentLikeTab();
    }

    var targetVideos = this.videos;
    if (__wx_is_account_like_page__()) {
      if (!this.likeTabVideos[this.currentLikeTabKey]) {
        this.likeTabVideos[this.currentLikeTabKey] = [];
      }
      targetVideos = this.likeTabVideos[this.currentLikeTabKey];
      this.videos = targetVideos;
    }

    // 限制最多300个视频
    if (targetVideos.length >= this._maxVideos) {
      if (targetVideos.length === this._maxVideos) {
        __wx_log({ msg: '⚠️ [Profile] 已达到最大采集数量 ' + this._maxVideos + ' 个' });
      }
      return;
    }

    // 清理标题
    if (videoData.title) {
      videoData.title = this.cleanHtmlTags(videoData.title);
    }

    // 检查是否已存在
    var exists = targetVideos.some(function (v) { return v.id === videoData.id; });
    if (!exists) {
      targetVideos.push(videoData);
      console.log('[Profile] 新增视频:', (videoData.title || '').substring(0, 30));

      // 每10个视频发送一次日志
      var filteredVideos = this.filterLivePictureVideos(targetVideos);
      var videoCount = filteredVideos.filter(function (v) { return v && v.type === 'media'; }).length;
      var liveReplayCount = filteredVideos.filter(function (v) { return v && v.type === 'live_replay'; }).length;

      if (videoCount > 0 && videoCount % 10 === 0 && videoCount !== this._lastTipVideoCount) {
        this._lastTipVideoCount = videoCount;
        var msg = '📊 [Profile] 已采集 ' + videoCount + ' 个视频';
        if (liveReplayCount > 0) msg += ', ' + liveReplayCount + ' 个直播回放';
        __wx_log({ msg: msg });
      }

      // 更新UI（使用通用批量下载组件，包含视频和直播回放）
      if (window.__wx_batch_download_manager__ && window.__wx_batch_download_manager__.isVisible) {
        var filteredVideos = this.filterLivePictureVideos(targetVideos).filter(function (v) {
          return v && (v.type === 'media' || v.type === 'live_replay');
        });
        var title = __wx_is_account_like_page__()
          ? ('赞和收藏 - ' + this.currentLikeTabLabel)
          : __wx_profile_list_page_title__();
        __update_batch_download_ui__(filteredVideos, title);
      }
    }
  }
};

// ==================== 事件监听 ====================

// 监听用户视频列表加载
WXE.onUserFeedsLoaded(function (feeds) {
  console.log('[Profile] onUserFeedsLoaded 事件触发，feeds:', feeds);

  if (!feeds || !Array.isArray(feeds)) {
    console.warn('[Profile] feeds 不是数组或为空');
    return;
  }

  var isListPage = __wx_is_profile_like_list_page__();
  console.log('[Profile] 是否是列表页:', isListPage, '当前路径:', window.location.pathname);
  if (!isListPage) return;

  console.log('[Profile] 开始处理', feeds.length, '个视频');

  var processedCount = 0;
  feeds.forEach(function (item) {
    if (!item || !item.objectDesc) {
      console.warn('[Profile] 跳过无效项:', item);
      return;
    }

    var media = item.objectDesc.media && item.objectDesc.media[0];
    if (!media) {
      console.warn('[Profile] 跳过无media的项:', item);
      return;
    }

    // 使用 WXU.format_feed 格式化数据
    var profile = WXU.format_feed(item);
    if (!profile) {
      console.warn('[Profile] format_feed 返回 null:', item);
      return;
    }

    // 传递给 collector
    window.__wx_channels_profile_collector.addVideoFromAPI(profile);
    processedCount++;
  });

  console.log('[Profile] 成功处理', processedCount, '个视频');
});

// 监听直播回放列表加载
WXE.onUserLiveReplayLoaded(function (feeds) {
  if (!feeds || !Array.isArray(feeds)) return;

  if (!__wx_is_profile_page__()) return;

  __wx_log({ msg: '📺 [Profile] 获取到直播回放列表，数量: ' + feeds.length });

  feeds.forEach(function (item) {
    if (!item || !item.objectDesc) return;

    var media = item.objectDesc.media && item.objectDesc.media[0];
    var liveInfo = item.liveInfo || {};

    // 获取时长
    var duration = 0;
    if (media && media.spec && media.spec.length > 0 && media.spec[0].durationMs) {
      duration = media.spec[0].durationMs;
    } else if (liveInfo.duration) {
      duration = liveInfo.duration;
    }

    // 构建直播回放数据
    var profile = {
      type: "live_replay",
      id: item.id,
      nonce_id: item.objectNonceId,
      title: window.__wx_channels_profile_collector.cleanHtmlTags(item.objectDesc.description || ''),
      coverUrl: media ? (media.thumbUrl || media.coverUrl || '') : '',
      thumbUrl: media ? (media.thumbUrl || '') : '',
      url: media ? (media.url + (media.urlToken || '')) : '',
      originalUrl: media ? (media.url || '') : '',
      urlToken: media ? (media.urlToken || '') : '',
      size: media ? (media.fileSize || 0) : 0,
      key: media
        ? __wx_channels_select_decrypt_key__(media.decodeKey, media.decryptKey, item.decodeKey, item.decryptKey)
        : __wx_channels_select_decrypt_key__(item.decodeKey, item.decryptKey),
      decryptKey: media
        ? __wx_channels_select_decrypt_key__(media.decryptKey, media.decodeKey, item.decryptKey, item.decodeKey)
        : __wx_channels_select_decrypt_key__(item.decryptKey, item.decodeKey),
      duration: duration,
      spec: media ? media.spec : [],
      nickname: item.contact ? item.contact.nickname : '',
      contact: item.contact || {},
      createtime: item.createtime || 0,
      liveInfo: liveInfo
    };

    // 传递给 collector
    window.__wx_channels_profile_collector.addVideoFromAPI(profile);
  });

  __wx_log({ msg: '✅ [Profile] 直播回放列表采集完成，共 ' + feeds.length + ' 个' });
});

// 监听赞和收藏/喜欢列表加载
WXE.onInteractionedFeedsLoaded(function (payload) {
  var feeds = Array.isArray(payload) ? payload : (payload && payload.feeds ? payload.feeds : []);
  var tabFlag = payload && payload.tabFlag != null ? payload.tabFlag : null;
  console.log('[Profile] onInteractionedFeedsLoaded 事件触发，feeds:', feeds, 'tabFlag:', tabFlag);

  if (!__wx_is_account_like_page__()) return;
  if (!feeds || !Array.isArray(feeds)) {
    console.warn('[Profile] interactioned feeds 不是数组或为空');
    return;
  }

  var collector = window.__wx_channels_profile_collector;
  var originalKey = collector.currentLikeTabKey;
  var originalLabel = collector.currentLikeTabLabel;
  if (tabFlag != null) {
    var mappedKey = collector.resolveLikeTabKeyByFlag(tabFlag);
    collector.likeTabFlags[mappedKey] = tabFlag;
    collector.currentLikeTabKey = mappedKey;
    collector.currentLikeTabLabel = __wx_get_like_label_by_key__(mappedKey);
    if (!collector.likeTabVideos[mappedKey]) {
      collector.likeTabVideos[mappedKey] = [];
    }
    collector.videos = collector.likeTabVideos[mappedKey];
  } else {
    collector.syncCurrentLikeTab();
  }

  var processedCount = 0;
  feeds.forEach(function (item) {
    var profile = WXU.format_feed(item);
    if (!profile) return;
    collector.addVideoFromAPI(profile);
    processedCount++;
  });

  collector.currentLikeTabKey = originalKey;
  collector.currentLikeTabLabel = originalLabel;
  if (collector.likeTabVideos[originalKey]) {
    collector.videos = collector.likeTabVideos[originalKey];
  }

  console.log('[Profile] 成功处理', processedCount, '个赞和收藏视频');
});

// ==================== 初始化 ====================

// 检查是否是列表页
function is_profile_page() {
  return __wx_is_profile_like_list_page__();
}

// 页面加载后初始化
if (is_profile_page()) {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () {
      window.__wx_channels_profile_collector.init();
    });
  } else {
    window.__wx_channels_profile_collector.init();
  }
}
