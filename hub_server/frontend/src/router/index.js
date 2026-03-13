import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '../store/user'

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/login',
            name: 'Login',
            component: () => import('../views/Login.vue'),
            meta: { layout: 'Auth' }
        },
        {
            path: '/register',
            name: 'Register',
            component: () => import('../views/Register.vue'),
            meta: { layout: 'Auth' }
        },
        {
            path: '/',
            redirect: '/dashboard'
        },
        {
            path: '/dashboard',
            name: 'Dashboard',
            component: () => import('../views/Dashboard.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/search',
            name: 'Search',
            component: () => import('../views/Search.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },

        {
            path: '/settings',
            name: 'Settings',
            component: () => import('../views/Settings.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/channel/:username',
            name: 'ChannelProfile',
            component: () => import('../views/ChannelProfile.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/subscriptions',
            name: 'Subscriptions',
            component: () => import('../views/Subscriptions.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/subscriptions/:id/videos',
            name: 'SubscriptionVideos',
            component: () => import('../views/SubscriptionVideos.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/tasks',
            name: 'Tasks',
            component: () => import('../views/Tasks.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/devices',
            name: 'Devices',
            component: () => import('../views/Devices.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/sync',
            name: 'Sync',
            component: () => import('../views/Sync.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/nodes/:id',
            name: 'NodeDetail',
            component: () => import('../views/NodeDetail.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        },
        {
            path: '/monitoring',
            name: 'Monitoring',
            component: () => import('../views/Monitoring.vue'),
            meta: {
                requiresAuth: true,
                requiresAdmin: true,  // 添加管理员权限要求
                layout: 'Sidebar'
            }
        },
        {
            path: '/admin',
            name: 'Admin',
            component: () => import('../views/Admin.vue'),
            meta: {
                requiresAuth: true,
                requiresAdmin: true,
                layout: 'Sidebar'
            }
        },
        {
            path: '/video/:id',
            name: 'VideoDetail',
            component: () => import('../views/VideoDetail.vue'),
            meta: { requiresAuth: true, layout: 'Sidebar' }
        }
    ]
})

router.beforeEach(async (to, from, next) => {
    const userStore = useUserStore()

    // Init auth if needed
    // Init auth if needed
    if (userStore.token && !userStore.user) {
        await userStore.initAuth()
    }

    if (to.meta.requiresAuth && !userStore.isAuthenticated) {
        next('/login')
        return
    }

    if (to.meta.requiresAdmin && userStore.user?.role !== 'admin') {
        next('/dashboard') // Redirect non-admins
        return
    }

    if (!to.meta.requiresAuth && userStore.isAuthenticated) {
        if (to.path === '/login' || to.path === '/register') {
            next('/dashboard')
            return
        }
    }

    next()
})

export default router
