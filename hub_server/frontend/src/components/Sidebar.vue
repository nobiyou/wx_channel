<template>
  <aside class="w-64 bg-surface-0 border-r border-surface-200 flex flex-col shrink-0 h-screen fixed left-0 top-0 z-50 transition-transform duration-300" :class="[visible ? 'translate-x-0' : '-translate-x-full']">
    <!-- Logo -->
    <div class="px-6 py-6 flex items-center gap-3 shrink-0">
      <div class="w-10 h-10 rounded-xl bg-primary text-primary-contrast flex items-center justify-center shadow-md">
        <i class="pi pi-bolt text-xl"></i>
      </div>
      <h2 class="font-serif text-xl font-bold text-text tracking-tight whitespace-nowrap">Hub Control</h2>
    </div>

    <!-- Navigation -->
    <div class="flex-1 overflow-y-auto px-3 py-2 scrollbar-hide">
        <Menu :model="items" class="w-full border-none !bg-transparent">
            <template #item="{ item, props }">
                <router-link v-if="item.route" :to="item.route" custom v-slot="{ href, navigate, isActive, isExactActive }">
                    <a :href="href" @click="navigate" class="flex items-center gap-2 px-4 py-3 rounded-xl transition-colors hover:bg-surface-100 text-text-muted hover:text-text cursor-pointer relative group" :class="{ '!bg-primary/10 !text-primary font-medium': isActive }">
                        <span :class="[item.icon, 'text-lg shrink-0']" />
                        <span class="font-medium whitespace-nowrap">{{ item.label }}</span>
                    </a>
                </router-link>
                <a v-else :href="item.url" v-bind="props.action" class="flex items-center gap-2 px-4 py-3 cursor-pointer relative group">
                     <span :class="[item.icon, 'text-lg shrink-0']" />
                    <span class="font-medium whitespace-nowrap">{{ item.label }}</span>
                </a>
            </template>
            <template #submenulabel="{ item }">
                <span class="text-xs font-bold text-text-muted uppercase tracking-widest px-2 mt-4 mb-2 block font-sans whitespace-nowrap">{{ item.label }}</span>
            </template>
        </Menu>
    </div>

    <!-- Footer Status -->
    <div class="p-4 border-t border-surface-200 shrink-0">
      <div class="flex justify-between items-center bg-surface-50 border border-surface-100 rounded-xl p-3">
        <span class="text-xs font-bold text-text-muted uppercase whitespace-nowrap">Status</span>
        <Tag severity="success" value="Online" icon="pi pi-circle-fill" rounded class="!text-xs"></Tag>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useUserStore } from '../store/user'
import Menu from 'primevue/menu'
import Tag from 'primevue/tag'

const props = defineProps({
    visible: {
        type: Boolean,
        default: true
    }
})

const route = useRoute()
const userStore = useUserStore()

const isActive = (path) => {
    if (path === '/') return route.path === '/'
    return route.path.startsWith(path)
}

const items = computed(() => {
    const menu = [
        {
            label: 'Core',
            items: [
                { label: '在线终端', icon: 'pi pi-desktop', route: '/dashboard' },
                { label: '穿透搜索', icon: 'pi pi-globe', route: '/search' }
            ]
        },
        {
            label: 'Content',
            items: [
                 { label: '订阅管理', icon: 'pi pi-bookmark', route: '/subscriptions' }
            ]
        },
        {
            label: 'Management',
            items: [
                { label: '设备管理', icon: 'pi pi-mobile', route: '/devices' },
                { label: '数据同步', icon: 'pi pi-sync', route: '/sync' },
                { label: '任务追踪', icon: 'pi pi-list', route: '/tasks' }
            ]
        },
        {
            label: 'Settings',
            items: [
                { label: '个人中心', icon: 'pi pi-user', route: '/settings' }
            ]
        }
    ]

    if (userStore.user?.role === 'admin') {
        menu.push({
            label: 'Admin',
            items: [
                { label: '系统管理', icon: 'pi pi-cog', route: '/admin' },
                { label: '系统监控', icon: 'pi pi-chart-line', route: '/monitoring' }
            ]
        })
    }

    return menu
})
</script>
