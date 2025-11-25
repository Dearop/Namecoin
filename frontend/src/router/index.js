import { createRouter, createWebHistory } from 'vue-router';
import Home from '../views/Home.vue';
import Wallet from '../views/Wallet.vue';

const routes = [
  {
    path: '/',
    name: 'Home',
    component: Home,
    meta: { title: 'Connect to Peer' }
  },
  {
    path: '/wallet',
    name: 'Wallet',
    component: Wallet,
    meta: { title: 'Peerster Wallet' },
    beforeEnter: (to, from, next) => {
      // Check if proxy address is set
      const proxyAddr = localStorage.getItem('proxyAddr');
      if (!proxyAddr) {
        next('/');
      } else {
        next();
      }
    }
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

// Update page title on route change
router.afterEach((to) => {
  document.title = to.meta.title || 'Peerster Wallet';
});

export default router;
