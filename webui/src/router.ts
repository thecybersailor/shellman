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
    ],
})

export default router
