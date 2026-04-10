// ============================================
// Video Preview and Player Functions - Requirements: 8.1, 8.2, 8.3, 8.4
// ============================================

// Video player state
const videoPlayerState = {
    isPlaying: false,
    isMuted: false,
    isFullscreen: false,
    currentVideo: null,
    controlsTimeout: null
};

// ============================================
// Thumbnail Component - Requirements: 8.1
// ============================================

/**
 * Create a thumbnail component HTML
 * @param {Object} options - Thumbnail options
 * @param {string} options.coverUrl - Cover image URL
 * @param {number} options.duration - Video duration in seconds
 * @param {string} options.videoId - Video ID for click handling
 * @param {string} options.videoUrl - Video URL for playback
 * @param {string} options.title - Video title
 * @param {string} options.filePath - Local file path (for downloaded videos)
 * @param {boolean} options.isDownloaded - Whether the video is downloaded
 * @returns {string} HTML string for the thumbnail
 */
function createThumbnailComponent(options) {
    const { coverUrl, duration, videoId, videoUrl, title, filePath, isDownloaded } = options;
    const durationText = duration ? formatDuration(duration) : '';
    
    // Determine click action based on whether video is downloaded
    const clickAction = isDownloaded && filePath 
        ? `openVideoPlayer('${escapeHtml(filePath)}', '${escapeHtml(title || '')}', true)`
        : coverUrl 
            ? `openLightbox('${escapeHtml(coverUrl)}', '${escapeHtml(title || '')}', '${durationText}')`
            : '';
    
    const thumbnailContent = coverUrl 
        ? `<img src="${escapeHtml(coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">`
        : '';
    
    return `
        <div class="video-thumbnail" onclick="${clickAction}">
            ${thumbnailContent}
            <div class="video-thumbnail-placeholder" ${coverUrl ? 'style="display:none"' : ''}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polygon points="23 7 16 12 23 17 23 7"/>
                    <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
                </svg>
            </div>
            <div class="video-thumbnail-overlay">
                <div class="video-thumbnail-play-btn">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polygon points="5 3 19 12 5 21 5 3"/>
                    </svg>
                </div>
            </div>
            ${durationText ? `<span class="video-thumbnail-duration">${durationText}</span>` : ''}
        </div>
    `;
}

/**
 * Render a simple thumbnail image (for tables and lists)
 * @param {string} coverUrl - Cover image URL
 * @param {string} title - Video title for alt text
 * @param {Function} onClick - Click handler
 * @returns {string} HTML string
 */
function renderThumbnailImage(coverUrl, title, onClick) {
    if (coverUrl) {
        return `
            <img class="table-thumbnail" 
                 src="${escapeHtml(coverUrl)}" 
                 alt="${escapeHtml(title || '')}"
                 onclick="${onClick || ''}"
                 onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
            <div class="table-thumbnail-placeholder" style="display:none">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polygon points="23 7 16 12 23 17 23 7"/>
                    <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
                </svg>
            </div>
        `;
    }
    return `
        <div class="table-thumbnail-placeholder">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polygon points="23 7 16 12 23 17 23 7"/>
                <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
            </svg>
        </div>
    `;
}

// ============================================
// Lightbox for Cover Image Preview - Requirements: 8.3
// ============================================

/**
 * Open the lightbox to preview a cover image
 * @param {string} imageUrl - URL of the image to display
 * @param {string} title - Title to display below the image
 * @param {string} meta - Additional metadata (e.g., duration)
 */
function openLightbox(imageUrl, title, meta) {
    if (!imageUrl) {
        showMessage('没有可预览的图片', 'info');
        return;
    }

    const overlay = document.getElementById('lightboxOverlay');
    const image = document.getElementById('lightboxImage');
    const titleEl = document.getElementById('lightboxTitle');
    const metaEl = document.getElementById('lightboxMeta');

    // Set content
    image.src = imageUrl;
    titleEl.textContent = title || '';
    metaEl.textContent = meta || '';

    // Show lightbox
    overlay.classList.add('active');

    // Add keyboard listener for ESC
    document.addEventListener('keydown', handleLightboxKeydown);

    // Prevent body scroll
    document.body.style.overflow = 'hidden';
}

/**
 * Close the lightbox
 * @param {Event} event - Click event (optional)
 */
function closeLightbox(event) {
    if (event) {
        event.stopPropagation();
    }

    const overlay = document.getElementById('lightboxOverlay');
    overlay.classList.remove('active');

    // Remove keyboard listener
    document.removeEventListener('keydown', handleLightboxKeydown);

    // Restore body scroll
    document.body.style.overflow = '';

    // Clear image source after animation
    setTimeout(() => {
        document.getElementById('lightboxImage').src = '';
    }, 300);
}

/**
 * Handle keyboard events for lightbox
 * @param {KeyboardEvent} event
 */
function handleLightboxKeydown(event) {
    if (event.key === 'Escape') {
        closeLightbox();
    }
}

// ============================================
// Video Player Modal - Requirements: 8.2, 8.4
// ============================================

/**
 * Open the video player modal
 * @param {string} videoSource - URL or file path of the video
 * @param {string} title - Video title
 * @param {boolean} isLocalFile - Whether the source is a local file
 */
function openVideoPlayer(videoSource, title, isLocalFile = false) {
    if (!videoSource) {
        showMessage('没有可播放的视频', 'error');
        return;
    }

    const modal = document.getElementById('videoPlayerModal');
    const video = document.getElementById('videoPlayer');
    const titleEl = document.getElementById('videoPlayerTitle');
    const loadingEl = document.getElementById('videoPlayerLoading');
    const errorEl = document.getElementById('videoPlayerError');

    // Reset state
    videoPlayerState.isPlaying = false;
    videoPlayerState.currentVideo = { source: videoSource, title: title, isLocal: isLocalFile };

    // Set title
    titleEl.textContent = title || '视频播放';

    // Show loading state
    loadingEl.style.display = 'flex';
    errorEl.style.display = 'none';
    
    // 确保按钮初始状态为播放图标
    updatePlayPauseButton();

    // Set video source
    if (isLocalFile) {
        // For local files, we need to use a special API endpoint
        // The backend should serve the file through an API
        video.src = `${ConnectionManager.serviceUrl}/api/video/stream?path=${encodeURIComponent(videoSource)}`;
    } else {
        video.src = videoSource;
    }

    // Show modal
    modal.classList.add('active');

    // Add event listeners
    video.addEventListener('loadedmetadata', handleVideoLoaded);
    video.addEventListener('error', handleVideoError);
    video.addEventListener('timeupdate', updateVideoProgress);
    video.addEventListener('ended', handleVideoEnded);
    video.addEventListener('play', handleVideoPlay);
    video.addEventListener('pause', handleVideoPause);

    // Add keyboard listener
    document.addEventListener('keydown', handleVideoPlayerKeydown);

    // Prevent body scroll
    document.body.style.overflow = 'hidden';

    // Setup controls visibility
    setupControlsVisibility();

    // Load the video
    video.load();
}

/**
 * Close the video player modal
 */
function closeVideoPlayer() {
    const modal = document.getElementById('videoPlayerModal');
    const video = document.getElementById('videoPlayer');

    // Pause and reset video
    video.pause();
    video.currentTime = 0;

    // Remove event listeners
    video.removeEventListener('loadedmetadata', handleVideoLoaded);
    video.removeEventListener('error', handleVideoError);
    video.removeEventListener('timeupdate', updateVideoProgress);
    video.removeEventListener('ended', handleVideoEnded);
    video.removeEventListener('play', handleVideoPlay);
    video.removeEventListener('pause', handleVideoPause);

    // Remove keyboard listener
    document.removeEventListener('keydown', handleVideoPlayerKeydown);

    // Exit fullscreen if active
    if (videoPlayerState.isFullscreen) {
        exitFullscreen();
    }

    // Hide modal
    modal.classList.remove('active');

    // Restore body scroll
    document.body.style.overflow = '';

    // Clear video source after animation
    setTimeout(() => {
        video.src = '';
        videoPlayerState.currentVideo = null;
    }, 300);

    // Clear controls timeout
    if (videoPlayerState.controlsTimeout) {
        clearTimeout(videoPlayerState.controlsTimeout);
    }
}

/**
 * Handle video loaded event
 */
function handleVideoLoaded() {
    const loadingEl = document.getElementById('videoPlayerLoading');
    loadingEl.style.display = 'none';
    updateVideoTime();
    // 确保按钮状态正确（视频加载完成时是暂停状态）
    videoPlayerState.isPlaying = false;
    updatePlayPauseButton();
}

/**
 * Handle video error event
 */
function handleVideoError() {
    const loadingEl = document.getElementById('videoPlayerLoading');
    const errorEl = document.getElementById('videoPlayerError');
    const errorText = document.getElementById('videoPlayerErrorText');

    loadingEl.style.display = 'none';
    errorEl.style.display = 'flex';

    const video = document.getElementById('videoPlayer');
    let errorMessage = '无法加载视频';

    if (video.error) {
        switch (video.error.code) {
            case MediaError.MEDIA_ERR_ABORTED:
                errorMessage = '视频加载被中止';
                break;
            case MediaError.MEDIA_ERR_NETWORK:
                errorMessage = '网络错误，无法加载视频';
                break;
            case MediaError.MEDIA_ERR_DECODE:
                errorMessage = '视频解码错误';
                break;
            case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
                errorMessage = '不支持的视频格式';
                break;
        }
    }

    errorText.textContent = errorMessage;
}

/**
 * Handle video ended event
 */
function handleVideoEnded() {
    videoPlayerState.isPlaying = false;
    updatePlayPauseButton();
}

/**
 * Handle video play event
 */
function handleVideoPlay() {
    videoPlayerState.isPlaying = true;
    updatePlayPauseButton();
}

/**
 * Handle video pause event
 */
function handleVideoPause() {
    videoPlayerState.isPlaying = false;
    updatePlayPauseButton();
}

/**
 * Toggle play/pause - Requirements: 8.4
 */
function togglePlayPause() {
    const video = document.getElementById('videoPlayer');

    if (video.paused) {
        video.play().catch(e => {
            console.error('Failed to play video:', e);
            showMessage('无法播放视频', 'error');
        });
    } else {
        video.pause();
    }
}

/**
 * Update play/pause button icon
 */
function updatePlayPauseButton() {
    const playIcon = document.getElementById('playIcon');
    const pauseIcon = document.getElementById('pauseIcon');

    if (videoPlayerState.isPlaying) {
        playIcon.style.display = 'none';
        pauseIcon.style.display = 'block';
    } else {
        playIcon.style.display = 'block';
        pauseIcon.style.display = 'none';
    }
}

/**
 * Seek video to position - Requirements: 8.4
 * @param {MouseEvent} event
 */
function seekVideo(event) {
    const video = document.getElementById('videoPlayer');
    const progressContainer = document.getElementById('videoProgressContainer');
    const rect = progressContainer.getBoundingClientRect();
    const pos = (event.clientX - rect.left) / rect.width;
    video.currentTime = pos * video.duration;
}

/**
 * Update video progress bar
 */
function updateVideoProgress() {
    const video = document.getElementById('videoPlayer');
    const progressBar = document.getElementById('videoProgressBar');

    if (video.duration) {
        const progress = (video.currentTime / video.duration) * 100;
        progressBar.style.width = `${progress}%`;
    }

    updateVideoTime();
}

/**
 * Update video time display
 */
function updateVideoTime() {
    const video = document.getElementById('videoPlayer');
    const timeEl = document.getElementById('videoTime');

    const current = formatVideoTime(video.currentTime);
    const duration = formatVideoTime(video.duration || 0);
    timeEl.textContent = `${current} / ${duration}`;
}

/**
 * Format time for video player display
 * @param {number} seconds
 * @returns {string}
 */
function formatVideoTime(seconds) {
    if (isNaN(seconds) || !isFinite(seconds)) return '00:00';

    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);

    if (h > 0) {
        return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
    }
    return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
}

/**
 * Toggle mute - Requirements: 8.4
 */
function toggleMute() {
    const video = document.getElementById('videoPlayer');
    video.muted = !video.muted;
    videoPlayerState.isMuted = video.muted;
    updateVolumeButton();
}

/**
 * Change volume - Requirements: 8.4
 * @param {number} value - Volume value (0-1)
 */
function changeVolume(value) {
    const video = document.getElementById('videoPlayer');
    video.volume = parseFloat(value);

    if (video.volume === 0) {
        video.muted = true;
    } else if (video.muted) {
        video.muted = false;
    }

    videoPlayerState.isMuted = video.muted;
    updateVolumeButton();
}

/**
 * Update volume button icon
 */
function updateVolumeButton() {
    const volumeIcon = document.getElementById('volumeIcon');
    const muteIcon = document.getElementById('muteIcon');
    const volumeSlider = document.getElementById('volumeSlider');
    const video = document.getElementById('videoPlayer');

    if (videoPlayerState.isMuted || video.volume === 0) {
        volumeIcon.style.display = 'none';
        muteIcon.style.display = 'block';
    } else {
        volumeIcon.style.display = 'block';
        muteIcon.style.display = 'none';
    }

    volumeSlider.value = video.muted ? 0 : video.volume;
}

/**
 * Toggle fullscreen - Requirements: 8.4
 */
function toggleFullscreen() {
    if (videoPlayerState.isFullscreen) {
        exitFullscreen();
    } else {
        enterFullscreen();
    }
}

/**
 * Enter fullscreen mode
 */
function enterFullscreen() {
    const wrapper = document.getElementById('videoPlayerWrapper');

    if (wrapper.requestFullscreen) {
        wrapper.requestFullscreen();
    } else if (wrapper.webkitRequestFullscreen) {
        wrapper.webkitRequestFullscreen();
    } else if (wrapper.msRequestFullscreen) {
        wrapper.msRequestFullscreen();
    }

    videoPlayerState.isFullscreen = true;
    updateFullscreenButton();

    // Listen for fullscreen change
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
}

/**
 * Exit fullscreen mode
 */
function exitFullscreen() {
    if (document.exitFullscreen) {
        document.exitFullscreen();
    } else if (document.webkitExitFullscreen) {
        document.webkitExitFullscreen();
    } else if (document.msExitFullscreen) {
        document.msExitFullscreen();
    }

    videoPlayerState.isFullscreen = false;
    updateFullscreenButton();
}

/**
 * Handle fullscreen change event
 */
function handleFullscreenChange() {
    videoPlayerState.isFullscreen = !!(document.fullscreenElement || document.webkitFullscreenElement);
    updateFullscreenButton();

    if (!videoPlayerState.isFullscreen) {
        document.removeEventListener('fullscreenchange', handleFullscreenChange);
        document.removeEventListener('webkitfullscreenchange', handleFullscreenChange);
    }
}

/**
 * Update fullscreen button icon
 */
function updateFullscreenButton() {
    const fullscreenIcon = document.getElementById('fullscreenIcon');
    const exitFullscreenIcon = document.getElementById('exitFullscreenIcon');

    if (videoPlayerState.isFullscreen) {
        fullscreenIcon.style.display = 'none';
        exitFullscreenIcon.style.display = 'block';
    } else {
        fullscreenIcon.style.display = 'block';
        exitFullscreenIcon.style.display = 'none';
    }
}

/**
 * Handle keyboard events for video player
 * @param {KeyboardEvent} event
 */
function handleVideoPlayerKeydown(event) {
    const video = document.getElementById('videoPlayer');

    switch (event.key) {
        case 'Escape':
            if (videoPlayerState.isFullscreen) {
                exitFullscreen();
            } else {
                closeVideoPlayer();
            }
            break;
        case ' ':
        case 'k':
            event.preventDefault();
            togglePlayPause();
            break;
        case 'ArrowLeft':
            event.preventDefault();
            video.currentTime = Math.max(0, video.currentTime - 5);
            break;
        case 'ArrowRight':
            event.preventDefault();
            video.currentTime = Math.min(video.duration, video.currentTime + 5);
            break;
        case 'ArrowUp':
            event.preventDefault();
            video.volume = Math.min(1, video.volume + 0.1);
            updateVolumeButton();
            break;
        case 'ArrowDown':
            event.preventDefault();
            video.volume = Math.max(0, video.volume - 0.1);
            updateVolumeButton();
            break;
        case 'm':
            toggleMute();
            break;
        case 'f':
            toggleFullscreen();
            break;
    }
}

/**
 * Setup controls visibility (auto-hide)
 */
function setupControlsVisibility() {
    const wrapper = document.getElementById('videoPlayerWrapper');
    const controls = document.getElementById('videoControls');

    wrapper.addEventListener('mousemove', () => {
        controls.classList.add('visible');

        if (videoPlayerState.controlsTimeout) {
            clearTimeout(videoPlayerState.controlsTimeout);
        }

        videoPlayerState.controlsTimeout = setTimeout(() => {
            if (videoPlayerState.isPlaying) {
                controls.classList.remove('visible');
            }
        }, 3000);
    });

    wrapper.addEventListener('mouseleave', () => {
        if (videoPlayerState.isPlaying) {
            controls.classList.remove('visible');
        }
    });
}

/**
 * Preview video from browse record (opens lightbox for cover or player for downloaded)
 * @param {string} id - Record ID
 */
function previewBrowseVideo(id) {
    const record = browseState.records.find(r => r.id === id);
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }

    // For browse records, show cover image in lightbox
    if (record.coverUrl) {
        openLightbox(record.coverUrl, record.title, record.duration ? formatDuration(record.duration) : '');
    } else {
        showMessage('没有可预览的封面图片', 'info');
    }
}

/**
 * Play downloaded video
 * @param {string} id - Download record ID
 */
function playDownloadedVideo(id) {
    const record = downloadState.records.find(r => r.id === id);
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }

    if (record.status !== 'completed') {
        showMessage('视频尚未下载完成', 'info');
        return;
    }

    if (!record.filePath) {
        showMessage('文件路径不可用', 'error');
        return;
    }

    openVideoPlayer(record.filePath, record.title, true);
}

// ============================================
// WebSocket Event Handlers - Requirements: 10.6
// ============================================
WebSocketClient.onDownloadProgress((progress) => {
    // Update queue item progress in real-time - Requirements: 10.6
    if (currentPage === 'queue') {
        updateQueueItemProgress(progress);
    }
});

WebSocketClient.onQueueChange((change) => {
    if (currentPage === 'queue') {
        // Handle different queue change actions
        if (change.action === 'add' && change.item) {
            queueState.items.push(change.item);
            renderQueueList();
            updateQueueStats();
        } else if (change.action === 'remove' && change.item) {
            queueState.items = queueState.items.filter(i => i.id !== change.item.id);
            renderQueueList();
            updateQueueStats();
        } else if (change.queue) {
            // Full queue update
            queueState.items = change.queue;
            renderQueueList();
            updateQueueStats();
        } else {
            // Fallback: reload entire queue
            loadDownloadQueue();
        }
    }
});

WebSocketClient.onStatsUpdate((stats) => {
    if (currentPage === 'dashboard') {
        loadDashboardData();
    }
});

// ============================================
// Initialization
// ============================================
document.addEventListener('DOMContentLoaded', () => {
    // Initialize SPA router - Requirements: 6.2
    initRouter();
    
    // Load saved service URL
    const savedUrl = StorageManager.loadServiceUrl();
    document.getElementById('settingServiceUrl').value = savedUrl;
    
    // Connect to service
    ConnectionManager.connect(savedUrl);
    
    // Load initial page data when connected
    ConnectionManager.onStatusChange((status) => {
        if (status === 'connected') {
            loadPageData(currentPage);
        }
    });
});