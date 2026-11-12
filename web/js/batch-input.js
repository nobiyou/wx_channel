// Batch download JSON compatibility helpers.
//
// Profile exports and single-video raw exports intentionally have different
// envelopes. Normalize both here so the console sends the stable batch API
// shape instead of relying on the source page's field names and value types.
(function (root) {
    'use strict';

    function firstValue() {
        for (var i = 0; i < arguments.length; i++) {
            var value = arguments[i];
            if (value !== undefined && value !== null && value !== '') return value;
        }
        return '';
    }

    function asNumber(value) {
        if (typeof value === 'number' && Number.isFinite(value)) return value;
        if (typeof value !== 'string' || !value.trim()) return 0;
        var parsed = Number(value.replace(/,/g, '').trim());
        return Number.isFinite(parsed) ? parsed : 0;
    }

    function durationToMilliseconds(value) {
        if (typeof value === 'number') return Math.max(0, value);
        if (typeof value !== 'string') return 0;
        var text = value.trim();
        if (!text) return 0;
        if (/^\d+(?:\.\d+)?$/.test(text)) return Math.max(0, Number(text));
        var parts = text.split(':').map(Number);
        if (parts.some(function (part) { return !Number.isFinite(part); })) return 0;
        if (parts.length === 2) return Math.max(0, (parts[0] * 60 + parts[1]) * 1000);
        if (parts.length === 3) return Math.max(0, (parts[0] * 3600 + parts[1] * 60 + parts[2]) * 1000);
        return 0;
    }

    function mediaOf(item) {
        if (!item || typeof item !== 'object') return null;
        if (item.media && !Array.isArray(item.media)) return item.media;
        if (item.objectDesc && Array.isArray(item.objectDesc.media)) return item.objectDesc.media[0] || null;
        if (Array.isArray(item.media)) return item.media[0] || null;
        return null;
    }

    function joinURL(url, token) {
        var value = String(url || '').trim();
        var suffix = String(token || '').trim();
        if (!value || !suffix || /[?&](?:token|url_token)=/i.test(value)) return value;
        return value + (value.indexOf('?') >= 0 ? '&' : '?') + 'token=' + encodeURIComponent(suffix);
    }

    function toBatchVideo(item) {
        if (!item || typeof item !== 'object') return null;
        var media = mediaOf(item) || {};
        var contact = item.contact || {};
        var spec = Array.isArray(item.spec) ? item.spec : (Array.isArray(media.spec) ? media.spec : []);
        var firstSpec = spec[0] || {};
        var url = firstValue(
            item.url,
            item.videoUrl,
            item.videoURL,
            item.originalUrl,
            item.playUrl,
            item.play_url,
            media.url,
            media.videoUrl,
            media.playUrl,
            media.play_url
        );
        url = joinURL(url, firstValue(item.urlToken, item.url_token, media.urlToken, media.url_token, media.urltoken));

        var id = String(firstValue(item.id, item.videoId, item.objectId, item.objectNonceId, item.nonce_id) || '').trim();
        if (!id) return null;

        var title = String(firstValue(
            item.title,
            item.description,
            item.name,
            item.objectDesc && item.objectDesc.description,
            '无标题'
        ));
        var author = String(firstValue(
            item.author,
            item.authorName,
            item.nickname,
            contact.nickname,
            item.userName,
            '未知作者'
        ));
        var cover = firstValue(item.coverUrl, item.coverURL, item.thumbUrl, media.thumbUrl, media.coverUrl);
        var key = firstValue(item.key, item.decryptKey, item.decodeKey, media.decodeKey, media.decryptKey);
        var duration = firstValue(item.durationMs, item.duration, item.duration_ms, media.durationMs, media.videoPlayLen);
        var size = firstValue(item.size, item.fileSize, media.fileSize);
        var resolution = firstValue(item.resolution, firstSpec.resolution, firstSpec.fileFormat);
        var fileFormat = firstValue(item.fileFormat, firstSpec.fileFormat);

        return {
            id: id,
            title: title,
            url: String(url || ''),
            authorName: author,
            author: author,
            key: String(key || ''),
            decryptKey: String(key || ''),
            coverUrl: String(cover || ''),
            sourceUrl: String(firstValue(item.sourceUrl, item.pageUrl) || ''),
            headers: item.headers && typeof item.headers === 'object' ? item.headers : {},
            durationMs: durationToMilliseconds(duration),
            size: asNumber(size),
            resolution: String(resolution || ''),
            fileFormat: String(fileFormat || ''),
            pageSource: String(firstValue(item.pageSource, 'batch_console_profile_import'))
        };
    }

    function candidateItems(payload) {
        if (Array.isArray(payload)) return payload;
        if (!payload || typeof payload !== 'object') return [];
        var collections = [payload.videos, payload.items, payload.feeds, payload.records, payload.data && payload.data.videos];
        for (var i = 0; i < collections.length; i++) {
            if (Array.isArray(collections[i])) return collections[i];
        }
        // Current-video raw export: { profile, rawFeed }.
        if (payload.profile && typeof payload.profile === 'object') {
            return [Object.assign({}, payload.profile, {
                sourceUrl: firstValue(payload.profile.sourceUrl, payload.pageUrl)
            })];
        }
        if (payload.video && typeof payload.video === 'object') return [payload.video];
        if (payload.rawFeed && typeof payload.rawFeed === 'object') return [payload.rawFeed];
        if (payload.data && typeof payload.data === 'object' && !Array.isArray(payload.data)) return [payload.data];
        return [payload];
    }

    function normalizeBatchVideoPayload(payload) {
        var seen = Object.create(null);
        return candidateItems(payload).map(toBatchVideo).filter(function (item) {
            if (!item || seen[item.id]) return false;
            seen[item.id] = true;
            return true;
        });
    }

    function parseBatchVideoJSON(text) {
        var payload;
        try {
            payload = typeof text === 'string' ? JSON.parse(text) : text;
        } catch (error) {
            throw new Error('JSON 格式错误: ' + error.message);
        }
        var videos = normalizeBatchVideoPayload(payload);
        if (videos.length === 0) throw new Error('JSON 中没有可下载的视频数据');
        return videos;
    }

    root.normalizeBatchVideoPayload = normalizeBatchVideoPayload;
    root.parseBatchVideoJSON = parseBatchVideoJSON;
})(typeof window !== 'undefined' ? window : globalThis);
