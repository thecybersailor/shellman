import { createRouter, createWebHistory } from 'vue-router'
import MainView from './App.vue'

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/',
            name: 'home',
            component: MainView,
        },
        {
            path: '/sess/:sessionId',
            name: 'session',
            component: MainView,
            props: true,
        },
        {
            path: '/m/tasks',
            name: 'mobile-tasks',
            component: MainView,
        },
    ],
})

export default router
