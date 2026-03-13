<template>
    <div class="min-h-screen bg-bg p-4 lg:p-12 font-sans text-text">
        <header class="flex flex-col md:flex-row justify-between items-start md:items-center mb-6 lg:mb-12 gap-4">
            <div class="flex items-center gap-3 lg:gap-6 w-full md:w-auto">
                <Button icon="pi pi-arrow-left" rounded variant="text" aria-label="Back" @click="goBack" class="!w-10 !h-10 lg:!w-12 lg:!h-12 !text-text hover:!bg-surface-100 shrink-0" />
                
                <!-- Loading State -->
                <div class="flex items-center gap-3 lg:gap-4" v-if="loading && !subscription">
                    <Skeleton shape="circle" size="3rem" class="lg:w-16 lg:h-16 shadow-neu-sm"></Skeleton>
                    <div>
                        <Skeleton width="8rem" height="1.5rem" class="mb-2 lg:w-48 lg:h-8"></Skeleton>
                    </div>
                </div>

                <!-- Subscription Info -->
                <div class="flex items-center gap-3 lg:gap-4 flex-1 min-w-0" v-else-if="subscription">
                    <div class="w-12 h-12 lg:w-16 lg:h-16 rounded-full shadow-neu-sm p-0.5 lg:p-1 bg-surface-0 shrink-0">
                        <img :src="subscription.headUrl || placeholderImg" class="w-full h-full rounded-full object-cover" @error="onImgError">
                    </div>
                    <div class="min-w-0 flex-1">
                        <h2 class="font-serif font-bold text-lg lg:text-2xl text-text mb-0.5 truncate leading-tight">{{ subscription.nickname }} 的订阅视频</h2>
                        <div class="mt-0.5">
                            <Tag :value="`共 ${totalVideos} 个视频`" icon="pi pi-video" severity="secondary" rounded class="!text-[10px] lg:!text-xs scale-90 origin-left"></Tag>
                        </div>
                    </div>
                </div>
            </div>
            
            <div v-if="client" class="self-end md:self-auto px-3 py-1.5 lg:px-4 lg:py-2 rounded-xl bg-surface-0 shadow-sm border border-surface-200 text-primary font-medium flex items-center gap-2 max-w-full">
                <span class="text-[10px] lg:text-xs uppercase tracking-wider text-text-muted whitespace-nowrap">Connected to</span>
                <strong class="text-xs lg:text-sm truncate max-w-[150px] lg:max-w-none">{{ client.hostname }}</strong>
            </div>
            <div v-else class="self-end md:self-auto px-3 py-1.5 lg:px-4 lg:py-2 rounded-xl bg-yellow-50 border border-yellow-200 text-yellow-700 text-xs lg:text-sm flex items-center gap-2">
                <i class="pi pi-bolt text-xs"></i>
                <span>自动选择设备</span>
            </div>
        </header>

        <div class="w-full max-w-[1600px] mx-auto">
            <!-- Grid Skeleton Loading -->
            <div v-if="loading && videos.length === 0" class="grid grid-cols-2 md:grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3 lg:gap-6">
                <div v-for="i in 8" :key="i" class="bg-surface-0 rounded-xl lg:rounded-2xl overflow-hidden shadow-sm aspect-[9/16] w-full p-2 flex flex-col gap-2">
                    <Skeleton width="100%" height="100%" class="rounded-lg"></Skeleton>
                </div>
            </div>

            <!-- Video Grid using DataView -->
            <DataView v-else-if="videos.length > 0" :value="videos" layout="grid">
                <template #grid="{ items }">
                    <div class="grid grid-cols-2 md:grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3 lg:gap-6">
                        <div 
                            v-for="video in items" 
                            :key="video.id"
                            @click="playVideo(video)"
                            class="bg-surface-0 rounded-xl lg:rounded-2xl overflow-hidden shadow-sm border border-surface-100 cursor-pointer transition-all hover:-translate-y-1 hover:shadow-md hover:border-primary/30 group relative aspect-[9/16] w-full"
                        >
                            <!-- Cover Image -->
                            <img :src="ensureHttps(video.cover_url) || placeholderImg" class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-105" @error="onImgError">
                            
                            <!-- Gradient Overlay -->
                            <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-80"></div>

                            <!-- Play Icon Overlay -->
                            <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-300">
                                <div class="bg-primary/90 text-white p-2 lg:p-3 rounded-full backdrop-blur-sm shadow-xl transform scale-75 group-hover:scale-100 transition-transform">
                                    <i class="pi pi-play text-xl lg:text-2xl"></i>
                                </div>
                            </div>

                            <!-- Duration Badge -->
                            <div v-if="video.duration" class="absolute top-1.5 right-1.5 lg:top-2 lg:right-2 bg-black/60 backdrop-blur-sm text-white text-[10px] px-1.5 py-0.5 rounded-md font-medium flex items-center gap-1">
                               <i class="pi pi-clock text-[10px]"></i>
                               {{ formatDuration(video.duration) }}
                            </div>

                            <!-- Bottom Info -->
                            <div class="absolute bottom-0 left-0 right-0 p-2 lg:p-4 text-white">
                                <div class="font-bold text-xs lg:text-sm line-clamp-2 mb-1 lg:mb-2 leading-snug">{{ video.title || '无标题' }}</div>
                                <div class="flex items-center justify-between text-[10px] lg:text-xs text-white/70">
                                        <div class="flex items-center gap-1">
                                        <span class="flex items-center gap-0.5 lg:gap-1" v-if="video.like_count">
                                            <i class="pi pi-thumbs-up text-[8px] lg:text-[10px]"></i>
                                            {{ formatCount(video.like_count) }}
                                        </span>
                                        </div>
                                        <div>
                                            {{ formatDate(video.published_at) }}
                                        </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </template>
            </DataView>

            <!-- Load More Button -->
            <div v-if="hasMore && !loading" class="text-center mt-8 lg:mt-12 mb-8">
                <Button 
                    label="加载更多" 
                    icon="pi pi-refresh"
                    rounded
                    outlined
                    severity="secondary" 
                    size="small"
                    @click="loadMoreVideos"
                />
            </div>

            <div v-if="loading && videos.length > 0" class="flex justify-center p-4 lg:p-8">
                <i class="pi pi-spin pi-spinner text-xl lg:text-2xl text-primary"></i>
            </div>
            
            <!-- Empty State -->
            <div v-if="!loading && videos.length === 0" class="text-center p-8 lg:p-16 text-text-muted bg-surface-0 rounded-2xl lg:rounded-[2rem] shadow-sm">
                 <i class="pi pi-video text-4xl lg:text-6xl text-surface-200 mb-4 block"></i>
                 <p class="text-base lg:text-lg font-medium mb-2">暂无视频</p>
            </div>
        </div>

  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useClientStore } from '../store/client'
import { formatTime, formatDuration } from '../utils/format'

// PrimeVue Components
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import DataView from 'primevue/dataview'
import Skeleton from 'primevue/skeleton'

const router = useRouter()
const route = useRoute()
const clientStore = useClientStore()
const client = computed(() => clientStore.currentClient)

const subscription = ref(null)
const videos = ref([])
const loading = ref(false)
const currentPage = ref(1)
const totalVideos = ref(0)
const hasMore = ref(false)


// 使用 data URI 避免外部请求和混合内容问题
const placeholderImg = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100"%3E%3Crect fill="%23f1f5f9" width="100" height="100"/%3E%3Ctext x="50" y="50" font-family="sans-serif" font-size="14" fill="%2394a3b8" text-anchor="middle" dominant-baseline="middle"%3E暂无图片%3C/text%3E%3C/svg%3E'

// 确保 URL 使用 HTTPS 协议
const ensureHttps = (url) => {
  if (!url || url === placeholderImg) return url
  return url.replace(/^http:\/\//i, 'https://')
}

onMounted(() => {
  const subscriptionId = route.params.id
  subscription.value = {
    id: subscriptionId,
    nickname: route.query.nickname || '未知用户',
    headUrl: ensureHttps(route.query.headUrl || '')
  }
  loadVideos()
})

const loadVideos = async () => {
  loading.value = true
  try {
    const token = localStorage.getItem('token')
    const res = await fetch(`/api/subscriptions/${subscription.value.id}/videos?page=${currentPage.value}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    const data = await res.json()
    if (data.code === 0) {
      videos.value.push(...data.data.videos)
      totalVideos.value = data.data.total
      const totalPages = Math.ceil(data.data.total / 20) // pageSize = 20
      hasMore.value = currentPage.value < totalPages
    }
  } catch (e) {
    console.error('Failed to load videos:', e)
    alert('加载视频失败')
  } finally {
    loading.value = false
  }
}

const loadMoreVideos = () => {
  currentPage.value++
  loadVideos()
}

const playVideo = (video) => {
    // 检查是否有完整的视频数据（video_url 和 decrypt_key）
    if (video.video_url && video.decrypt_key) {
        console.log('[SubscriptionVideos] Playing video from saved data:', video.title)
        
        // 直接使用保存的数据跳转到视频详情页
        router.push({
            path: `/video/${video.object_id}`,
            query: {
                from: 'subscription' // 标记来源
            },
            state: {
                // 传递订阅视频的完整信息
                subscriptionVideo: {
                    id: video.object_id,
                    nonce_id: video.object_nonce_id,
                    title: video.title,
                    cover_url: video.cover_url,
                    video_url: video.video_url,
                    decrypt_key: video.decrypt_key,
                    duration: video.duration,
                    like_count: video.like_count,
                    comment_count: video.comment_count
                }
            }
        })
    } else if (video.object_id) {
        // 如果没有保存的视频数据，回退到原来的方式（请求 API）
        console.log('[SubscriptionVideos] Playing video via API:', video.title)
        router.push({
            name: 'VideoDetail',
            params: { id: video.object_id },
            query: { nonceId: video.object_nonce_id }
        })
    } else {
        console.error('Video item invalid:', video)
    }
}


const goBack = () => {
  router.push('/subscriptions')
}

const formatCount = (count) => {
  if (count >= 10000) {
    return (count / 10000).toFixed(1) + '万'
  }
  return count || 0
}

const formatDate = (dateStr) => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now - date
  const days = Math.floor(diff / 86400000)
  
  if (days === 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return `${days}天前`
  if (days < 30) return `${Math.floor(days / 7)}周前`
  if (days < 365) return `${Math.floor(days / 30)}个月前`
  return date.toLocaleDateString('zh-CN')
}

const onImgError = (e) => {
  e.target.src = placeholderImg
}
</script>
