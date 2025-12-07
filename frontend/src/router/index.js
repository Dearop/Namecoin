import { createRouter, createWebHistory } from 'vue-router';
import Wallet from '../views/Wallet.vue';

const routes = [
  {
    path: '/',
    name: 'Wallet',
    component: Wallet,
    meta: { title: 'Peerster Wallet' }
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
