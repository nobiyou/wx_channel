<template>
    <div class="min-h-screen bg-bg p-4 lg:p-12 font-sans text-text">
    <header class="flex flex-col md:flex-row justify-between items-start md:items-center mb-4 lg:mb-8 gap-4">
      <div class="flex items-center gap-3 lg:gap-6">
        <Button icon="pi pi-arrow-left" rounded variant="text" aria-label="Back" @click="goBack" class="!w-10 !h-10 lg:!w-12 lg:!h-12 !text-text hover:!bg-surface-100 shrink-0" />
        <h2 class="font-serif font-bold text-xl lg:text-2xl text-text">视频详情</h2>
      </div>
      
      <div v-if="client" class="self-end md:self-auto px-3 py-1.5 lg:px-4 lg:py-2 rounded-xl bg-surface-0 shadow-sm border border-surface-200 text-primary font-medium flex items-center gap-2">
        <span class="text-[10px] lg:text-xs uppercase tracking-wider text-text-muted">Connected to</span>
        <strong class="text-xs lg:text-sm truncate max-w-[150px] lg:max-w-none">{{ client.hostname }}</strong>
      </div>
      <div v-else class="self-end md:self-auto px-3 py-1.5 lg:px-4 lg:py-2 rounded-xl bg-yellow-50 border border-yellow-200 text-yellow-700 text-xs lg:text-sm flex items-center gap-2">
        <i class="pi pi-bolt text-xs lg:text-sm"></i>
        <span>自动选择设备</span>
      </div>
    </header>

    <!-- Loading State with Skeleton -->
    <div v-if="loading" class="max-w-[1600px] mx-auto grid grid-cols-1 lg:grid-cols-12 gap-4 lg:gap-8">
        <div class="lg:col-span-8 flex flex-col gap-4 lg:gap-6">
            <Skeleton height="300px" borderRadius="1rem" class="w-full lg:h-[450px] lg:rounded-[1.5rem]"></Skeleton>
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm">
                <Skeleton width="60%" height="1.5rem" class="mb-4 lg:h-2rem"></Skeleton>
                <div class="flex gap-4">
                    <Skeleton width="80px" height="1.2rem" class="lg:w-100px lg:h-1.5rem"></Skeleton>
                    <Skeleton width="80px" height="1.2rem" class="lg:w-100px lg:h-1.5rem"></Skeleton>
                </div>
            </div>
        </div>
        <div class="lg:col-span-4 space-y-4 lg:space-y-6">
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm flex gap-4">
                <Skeleton shape="circle" size="3rem" class="lg:w-4rem lg:h-4rem"></Skeleton>
                <div class="flex-1">
                    <Skeleton width="50%" height="1.2rem" class="mb-2 lg:h-1.5rem"></Skeleton>
                    <Skeleton width="80%"></Skeleton>
                </div>
            </div>
             <Skeleton height="150px" borderRadius="1rem" class="lg:h-[200px]"></Skeleton>
        </div>
    </div>

    <!-- Client Inactive State -->
    <div v-else-if="isClientInactive" class="flex flex-col items-center justify-center p-8 lg:p-12 text-center max-w-2xl mx-auto">
        <div class="w-16 h-16 lg:w-20 lg:h-20 bg-orange-50 rounded-full flex items-center justify-center mb-6">
            <i class="pi pi-lock text-orange-500 text-2xl lg:text-3xl"></i>
        </div>
        <h3 class="text-xl lg:text-2xl font-bold text-text mb-3">无法连接到视频号页面</h3>
        <p class="text-text-muted mb-8 lg:text-lg">请确保您的微信视频号页面处于打开且激活状态。</p>
        
        <div class="bg-surface-0 border border-surface-200 rounded-2xl p-6 lg:p-8 text-left w-full mb-8 shadow-sm">
            <h4 class="font-bold text-text mb-4">解决方案：</h4>
            <ol class="list-decimal list-inside space-y-3 text-text-muted text-sm lg:text-base">
                <li>在浏览器中打开 <strong class="text-text">微信视频号</strong> 页面。</li>
                <li>确保该页面没有被最小化，且处于当前浏览器的 <strong class="text-text">可见标签页</strong>。</li>
                <li>如果页面已打开，请尝试刷新该页面。</li>
                <li>完成后点击下方的 <strong class="text-primary">重试</strong> 按钮。</li>
            </ol>
        </div>

        <Button label="已解决，重试" icon="pi pi-refresh" size="large" @click="loadVideoDetail" />
    </div>

    <!-- Generic Error State -->
    <div v-else-if="error" class="flex flex-col items-center justify-center p-8 lg:p-12 text-center">
        <div class="w-16 h-16 bg-red-50 rounded-full flex items-center justify-center mb-4">
            <i class="pi pi-exclamation-triangle text-red-500 text-2xl"></i>
        </div>
        <h3 class="text-lg font-bold text-text mb-2">加载失败</h3>
        <Message severity="error" :closable="false" class="mb-4 text-sm lg:text-base max-w-lg">{{ error }}</Message>
        <div v-if="debugInfo" class="mt-4 p-4 bg-surface-100 rounded text-[10px] lg:text-xs font-mono whitespace-pre-wrap max-w-2xl overflow-auto text-left w-full h-32 lg:h-48 border border-surface-200">
            {{ debugInfo }}
        </div>
        <Button label="重试" icon="pi pi-refresh" @click="loadVideoDetail" class="mt-6" severity="secondary" />
    </div>

    <!-- Content -->
    <div v-else class="max-w-[1600px] mx-auto grid grid-cols-1 lg:grid-cols-12 gap-4 lg:gap-8">
        <!-- Left Column: Video & Basic Info -->
        <div class="lg:col-span-8 flex flex-col">
            <!-- Video Player -->
            <div class="bg-black rounded-xl lg:rounded-3xl overflow-hidden shadow-card aspect-video mb-4 lg:mb-6 relative group w-full">
                <video 
                    v-if="playerUrl" 
                    :src="playerUrl" 
                    controls 
                    autoplay 
                    class="w-full h-full"
                    controlsList="nodownload"
                    oncontextmenu="return false"
                ></video>
                <div v-else class="w-full h-full flex items-center justify-center text-white/50 flex-col gap-4">
                    <i class="pi pi-exclamation-circle text-3xl lg:text-4xl"></i>
                    <div class="text-sm lg:text-base">无法获取视频地址</div>
                     <div class="text-xs text-white/30 max-w-md text-center px-4">
                        可能原因：该视频已被加密或您无权访问。请尝试在微信中重新打开该视频。
                    </div>
                </div>
            </div>
            
            <!-- Basic Video Info -->
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm mb-4 lg:mb-6">
                <h1 class="text-lg lg:text-2xl font-bold text-text mb-2 lg:mb-3 leading-snug">{{ video.title || '无标题' }}</h1>
                <div class="flex items-center gap-2 lg:gap-4 text-text-muted text-xs lg:text-sm flex-wrap">
                     <Tag icon="pi pi-calendar" severity="secondary" rounded :value="formatDate(video.createTime)" class="!text-xs lg:!text-sm"></Tag>
                     <Tag v-if="video.ipRegion" icon="pi pi-map-marker" severity="secondary" rounded :value="video.ipRegion" class="!text-xs lg:!text-sm"></Tag>
                </div>
            </div>

            <!-- Description -->
            <div class="prose prose-sm text-text bg-surface-0 p-4 lg:p-6 rounded-2xl whitespace-pre-wrap leading-relaxed shadow-sm text-sm lg:text-base">
                {{ video.desc }}
            </div>
        </div>

        <!-- Right Column: Author, Stats, Resolutions -->
        <div class="lg:col-span-4 space-y-4 lg:space-y-6">
            <!-- Author Card -->
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm">
                <div class="flex items-start gap-3 lg:gap-4 mb-4">
                     <div class="w-12 h-12 lg:w-16 lg:h-16 rounded-full shadow-neu-sm p-1 shrink-0 overflow-hidden cursor-pointer hover:scale-105 transition-transform bg-surface-0" @click="goToChannel(video.author.username)">
                        <img :src="ensureHttps(video.author.headUrl)" class="w-full h-full rounded-full object-cover" @error="onImgError">
                     </div>
                     <div class="flex-1 min-w-0 pt-0.5 lg:pt-1">
                         <div class="flex items-center gap-2 mb-0.5 lg:mb-1">
                            <div class="font-bold text-base lg:text-lg text-text hover:text-primary cursor-pointer truncate" @click="goToChannel(video.author.username)">
                                {{ video.author.nickname }}
                            </div>
                         </div>
                         <div class="text-text-muted text-xs line-clamp-2">{{ video.author.signature || '暂无简介' }}</div>
                     </div>
                </div>
                <Button 
                    :label="subscribing ? '处理中...' : (isSubscribed ? '已订阅' : '订阅')" 
                    :icon="isSubscribed ? 'pi pi-check' : 'pi pi-plus'"
                    :disabled="subscribing"
                    :severity="isSubscribed ? 'secondary' : 'primary'"
                    class="w-full !p-2 lg:!p-3 !text-sm lg:!text-base"
                    @click="toggleSubscribe" 
                />
            </div>

            <!-- Interaction Stats -->
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm">
                <h3 class="font-bold text-text mb-3 lg:mb-4 text-xs lg:text-sm uppercase tracking-wider text-surface-400">互动数据</h3>
                <div class="grid grid-cols-2 gap-3 lg:gap-4">
                    <div class="flex items-center gap-2 lg:gap-3 p-2.5 lg:p-3 rounded-xl bg-surface-50 border border-surface-100">
                        <i class="pi pi-thumbs-up text-primary text-lg lg:text-xl"></i>
                        <div>
                            <div class="text-[10px] lg:text-xs text-text-muted">点赞</div>
                            <div class="font-bold text-sm lg:text-base text-text">{{ video.likeCount || 0 }}</div>
                        </div>
                    </div>
                     <div class="flex items-center gap-2 lg:gap-3 p-2.5 lg:p-3 rounded-xl bg-surface-50 border border-surface-100">
                        <i class="pi pi-heart text-red-500 text-lg lg:text-xl"></i>
                        <div>
                            <div class="text-[10px] lg:text-xs text-text-muted">收藏</div>
                            <div class="font-bold text-sm lg:text-base text-text">{{ video.favCount || 0 }}</div>
                        </div>
                    </div>
                     <div class="flex items-center gap-2 lg:gap-3 p-2.5 lg:p-3 rounded-xl bg-surface-50 border border-surface-100">
                        <i class="pi pi-share-alt text-blue-500 text-lg lg:text-xl"></i>
                        <div>
                            <div class="text-[10px] lg:text-xs text-text-muted">转发</div>
                            <div class="font-bold text-sm lg:text-base text-text">{{ video.forwardCount || 0 }}</div>
                        </div>
                    </div>
                     <div class="flex items-center gap-2 lg:gap-3 p-2.5 lg:p-3 rounded-xl bg-surface-50 border border-surface-100">
                        <i class="pi pi-comments text-green-500 text-lg lg:text-xl"></i>
                        <div>
                            <div class="text-[10px] lg:text-xs text-text-muted">评论</div>
                            <div class="font-bold text-sm lg:text-base text-text">{{ video.commentCount || 0 }}</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Resolutions / Formats -->
            <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm" v-if="video.specs && video.specs.length > 0">
                <h3 class="font-bold text-text mb-3 lg:mb-4 text-xs lg:text-sm uppercase tracking-wider text-surface-400">画质选择</h3>
                <div class="space-y-2">
                    <button 
                        v-for="(spec, index) in video.specs" 
                        :key="index"
                        @click="switchResolution(spec)"
                        class="w-full flex items-center justify-between px-3 py-2.5 lg:px-4 lg:py-3 rounded-xl border transition-all text-left group"
                        :class="currentSpec === spec ? 'border-primary bg-primary-60 dark:bg-transparent' : 'border-surface-100 dark:border-surface-800 hover:border-primary hover:bg-surface-50 dark:hover:bg-surface-800'"
                    >
                        <div class="flex flex-col">
                            <span class="font-medium text-xs lg:text-sm text-text group-hover:text-primary">
                                {{ spec.width }}x{{ spec.height }}
                            </span>
                            <span class="text-[10px] lg:text-xs text-text-muted">{{ spec.fileFormat }} · {{ (spec.bitRate / 1024).toFixed(1) }} Mbps</span>
                        </div>
                        <i v-if="currentSpec === spec" class="pi pi-check-circle text-primary text-sm lg:text-base"></i>
                    </button>
                </div>
            </div>
        </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useClientStore } from '../store/client'
import { formatTime, formatDuration } from '../utils/format'

// PrimeVue Components
import Button from 'primevue/button'
import Skeleton from 'primevue/skeleton'
import Tag from 'primevue/tag'
import Message from 'primevue/message'

const route = useRoute()
const router = useRouter()
const clientStore = useClientStore()

const client = computed(() => clientStore.currentClient)

const loading = ref(true) 
const error = ref(null)
const isClientInactive = ref(false) // New state for specific error
const debugInfo = ref('')

const video = ref({
    author: {}
})
const playerUrl = ref('')
const currentSpec = ref(null)

const isSubscribed = ref(false)
const subscribing = ref(false)
const subscriptionId = ref(null)

const placeholderImg = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100"%3E%3Crect fill="%23f1f5f9" width="100" height="100"/%3E%3Ctext x="50" y="50" font-family="sans-serif" font-size="14" fill="%2394a3b8" text-anchor="middle" dominant-baseline="middle"%3E暂无图片%3C/text%3E%3C/svg%3E'

const ensureHttps = (url) => {
    if (!url || url === placeholderImg) return url
    return url.replace(/^http:\/\//i, 'https://')
}

onMounted(() => {
    loadVideoDetail()
})

const loadVideoDetail = async () => {
    loading.value = true
    error.value = null
    isClientInactive.value = false
    debugInfo.value = ''
    
    try {
        // 检查是否从浏览记录跳转过来
        const browseRecord = history.state?.browseRecord
        if (browseRecord && route.query.from === 'browse_history') {
            console.log('[VideoDetail] Loading from browse history:', browseRecord)
            
            // 直接使用浏览记录中的数据
            video.value = {
                id: browseRecord.id,
                title: browseRecord.title,
                desc: browseRecord.title,
                createTime: Date.now(),
                author: {
                    username: browseRecord.author,
                    nickname: browseRecord.author,
                    headUrl: '',
                    signature: ''
                },
                baseUrl: browseRecord.video_url,
                urlToken: '',
                decryptKey: browseRecord.decrypt_key || '',
                readCount: 0,
                likeCount: browseRecord.like_count || 0,
                favCount: browseRecord.fav_count || 0,
                forwardCount: 0,
                commentCount: browseRecord.comment_count || 0,
                ipRegion: ''
            }
            
            // 构建完整的视频 URL
            let fullVideoUrl = video.value.baseUrl
            
            // 优先使用保存的 file_format 字段
            if (browseRecord.file_format) {
                console.log('[VideoDetail] Using saved file_format:', browseRecord.file_format)
                if (!fullVideoUrl.includes('X-snsvideoflag')) {
                    fullVideoUrl += `&X-snsvideoflag=${browseRecord.file_format}`
                }
            } else {
                // 如果没有保存 file_format，根据分辨率推测（向后兼容）
                console.log('[VideoDetail] No file_format saved, guessing from resolution:', browseRecord.resolution)
                let videoFormat = 'xWT111' // 默认格式
                if (browseRecord.resolution) {
                    const res = browseRecord.resolution.toLowerCase()
                    if (res.includes('1080') || res.includes('1920')) {
                        videoFormat = 'xWT128' // 1080p
                    } else if (res.includes('720') || res.includes('1280')) {
                        videoFormat = 'xWT111' // 720p
                    }
                }
                if (!fullVideoUrl.includes('X-snsvideoflag')) {
                    fullVideoUrl += `&X-snsvideoflag=${videoFormat}`
                }
            }
            
            if (fullVideoUrl && video.value.decryptKey) {
                // 构建通过 Hub Server 代理的 URL
                let finalUrl = `/api/video/play?url=${encodeURIComponent(fullVideoUrl)}`
                finalUrl += `&key=${video.value.decryptKey}`
                playerUrl.value = finalUrl
                console.log('[VideoDetail] Player URL:', finalUrl)
            } else if (fullVideoUrl) {
                // 如果没有解密密钥，直接使用原始 URL
                playerUrl.value = fullVideoUrl
            }
            
            loading.value = false
            return
        }
        
        // 检查是否从订阅视频跳转过来
        const subscriptionVideo = history.state?.subscriptionVideo
        if (subscriptionVideo && route.query.from === 'subscription') {
            console.log('[VideoDetail] Loading from subscription:', subscriptionVideo)
            
            // 直接使用订阅视频中的数据
            video.value = {
                id: subscriptionVideo.id,
                title: subscriptionVideo.title,
                desc: subscriptionVideo.title,
                createTime: Date.now(),
                author: {
                    username: '',
                    nickname: '',
                    headUrl: '',
                    signature: ''
                },
                baseUrl: subscriptionVideo.video_url,
                urlToken: '',
                decryptKey: subscriptionVideo.decrypt_key || '',
                readCount: 0,
                likeCount: subscriptionVideo.like_count || 0,
                favCount: 0,
                forwardCount: 0,
                commentCount: subscriptionVideo.comment_count || 0,
                ipRegion: ''
            }
            
            // 构建完整的视频 URL（订阅视频通常已经包含完整URL）
            let fullVideoUrl = video.value.baseUrl
            
            // 如果URL不包含 X-snsvideoflag，添加默认格式
            if (!fullVideoUrl.includes('X-snsvideoflag')) {
                fullVideoUrl += `&X-snsvideoflag=xWT128` // 默认使用高清格式
            }
            
            if (fullVideoUrl && video.value.decryptKey) {
                // 构建通过 Hub Server 代理的 URL
                let finalUrl = `/api/video/play?url=${encodeURIComponent(fullVideoUrl)}`
                finalUrl += `&key=${video.value.decryptKey}`
                playerUrl.value = finalUrl
                console.log('[VideoDetail] Player URL (subscription):', finalUrl)
            } else if (fullVideoUrl) {
                // 如果没有解密密钥，直接使用原始 URL
                playerUrl.value = fullVideoUrl
            }
            
            loading.value = false
            return
        }
        
        // 原有的加载逻辑
        if (!client.value) {
           await clientStore.fetchClients() 
           if (!clientStore.currentClient && clientStore.clients.length > 0) {
               clientStore.setCurrentClient(clientStore.clients[0].id)
           }
        }
        
        // Fetch Video Info
        const res = await clientStore.remoteCall('api_call', {
            key: 'key:channels:feed_profile',
            body: {
                objectId: route.params.id,
                nonceId: route.query.nonceId
            }
        })
        
        console.log('[VideoDetail] API Response:', res)
        debugInfo.value = JSON.stringify(res, null, 2)
        
        // Robust data extraction
        const dataNode = res.data || res
        const finderObject = dataNode.object || dataNode.data?.object || dataNode // Try multiple levels
        
        if (!finderObject || !finderObject.objectDesc) {
             throw new Error("Invalid video data structure: " + JSON.stringify(dataNode).substring(0, 100))
        }

        const desc = finderObject.objectDesc || {}
        const media = (desc.media && desc.media[0]) || {}
        
        video.value = {
            id: finderObject.id,
            title: desc.description,
            desc: desc.description,
            createTime: finderObject.createtime,
            author: {
                username: finderObject.username,
                nickname: finderObject.nickname,
                // Try finderObject.contact.headUrl first (as per user JSON), then fallback to direct property
                headUrl: ensureHttps(finderObject.contact?.headUrl || finderObject.headUrl),
                signature: finderObject.signature || finderObject.contact?.signature
            },
            // Handle urlToken logic
            baseUrl: media.url,
            urlToken: media.urlToken || '',
            
            readCount: finderObject.readCount || 0,
            likeCount: finderObject.likeCount || 0,
            favCount: finderObject.favCount || 0,
            forwardCount: finderObject.forwardCount || 0,
            commentCount: finderObject.commentCount || 0,
            // User JSON shows regionText
            ipRegion: finderObject.ipRegionInfo ? (finderObject.ipRegionInfo.regionText || finderObject.ipRegionInfo.desc) : (finderObject.ipRegion || '')
        }
        
        // Append urlToken if present
        if (video.value.baseUrl && video.value.urlToken) {
            video.value.baseUrl += video.value.urlToken
        }

        if (media.spec && Array.isArray(media.spec)) {
            // Sort by bitrate ASCENDING (Lowest quality first)
            video.value.specs = media.spec.sort((a,b) => a.bitRate - b.bitRate)
            if (video.value.specs.length > 0) {
                currentSpec.value = video.value.specs[0]
            }
        }

        video.value.decryptKey = media.decodeKey || ''
        
        if (video.value.baseUrl) {
             updatePlayerUrl(video.value.baseUrl, video.value.decryptKey, currentSpec.value ? currentSpec.value.fileFormat : null)
        } else {
             console.warn('[VideoDetail] Warning: baseUrl is empty', media)
        }

        checkSubscriptionStatus()

    } catch (err) {
        console.error('Failed to load video detail:', err)
        if (err.message && err.message.includes('客户端页面未激活')) {
            isClientInactive.value = true
            error.value = '无法连接到视频号页面'
        } else {
            error.value = '加载视频详情失败: ' + err.message
        }
        
        if (!debugInfo.value) {
            debugInfo.value = err.stack || err.message
        }
    } finally {
        loading.value = false
    }
}

const toggleSubscribe = async () => {
    subscribing.value = true
    try {
        const token = localStorage.getItem('token')
        
        if (isSubscribed.value) {
            if (!subscriptionId.value) return
            
            const res = await fetch(`/api/subscriptions/${subscriptionId.value}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            })
            
            if (res.ok) {
                isSubscribed.value = false
                subscriptionId.value = null
            } else {
                alert('取消订阅失败')
            }
        } else {
            const res = await fetch('/api/subscriptions', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify({
                    wx_username: video.value.author?.username,
                    wx_nickname: video.value.author?.nickname,
                    wx_head_url: video.value.author?.headUrl,
                    wx_signature: video.value.author?.signature
                })
            })
            
            const data = await res.json()
            if (data.code === 0) {
                isSubscribed.value = true
                subscriptionId.value = data.data.id
            } else {
                alert('订阅失败: ' + (data.message || ''))
            }
        }
    } catch (e) {
        console.error('Subscription error:', e)
        alert('操作失败: ' + e.message)
    } finally {
        subscribing.value = false
    }
}

const switchResolution = (spec) => {
    currentSpec.value = spec
    const fileFormat = spec.fileFormat
    updatePlayerUrl(video.value.baseUrl, video.value.decryptKey, fileFormat)
}

const updatePlayerUrl = (baseUrl, decryptKey, fileFormat) => {
    if (!baseUrl) return
    let url = baseUrl
    if (fileFormat) {
        url += `&X-snsvideoflag=${fileFormat}`
    }
    let finalUrl = `/api/video/play?url=${encodeURIComponent(url)}`
    if (decryptKey) finalUrl += `&key=${decryptKey}`
    
    playerUrl.value = finalUrl
}

const goToChannel = (username) => {
    if (!username) return
    router.push({
        name: 'ChannelProfile',
        params: { username: username },
        query: {
            username: username,
            nickname: video.value.author?.nickname,
            headUrl: video.value.author?.headUrl,
            signature: video.value.author?.signature
        }
    })
}

const formatDate = (timestamp) => {
    if (!timestamp) return ''
    return new Date(timestamp * 1000).toLocaleString('zh-CN', {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: 'numeric'
    })
}

const goBack = () => {
    router.go(-1)
}

const checkSubscriptionStatus = async () => {
    try {
        const token = localStorage.getItem('token')
        if (!token) return
        
        const res = await fetch('/api/subscriptions', {
            headers: { 'Authorization': `Bearer ${token}` }
        })
        const data = await res.json()
        if (data.code === 0) {
            const currentUsername = video.value.author?.username
            if (!currentUsername) return

            const subscription = (data.data || []).find(sub => sub.wx_username === currentUsername)
            if (subscription) {
                isSubscribed.value = true
                subscriptionId.value = subscription.id
            }
        }
    } catch (e) {
        console.error('Failed to check subscription status:', e)
    }
}

const onImgError = (e) => {
  e.target.src = placeholderImg
}
</script>
