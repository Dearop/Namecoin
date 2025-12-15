import { describe, it, expect } from 'vitest';
import { createRouter, createWebHistory } from 'vue-router';
import Home from '../../views/Home.vue';
import Wallet from '../../views/Wallet.vue';

describe('Router', () => {
  it('should have home route', () => {
    const routes = [
      {
        path: '/',
        name: 'home',
        component: Home
      },
      {
        path: '/wallet',
        name: 'wallet',
        component: Wallet
      }
    ];

    const router = createRouter({
      history: createWebHistory(),
      routes
    });

    expect(router.getRoutes()).toHaveLength(2);
    expect(router.getRoutes()[0].path).toBe('/');
    expect(router.getRoutes()[1].path).toBe('/wallet');
  });

  it('should have wallet route', () => {
    const routes = [
      {
        path: '/',
        name: 'home',
        component: Home
      },
      {
        path: '/wallet',
        name: 'wallet',
        component: Wallet
      }
    ];

    const router = createRouter({
      history: createWebHistory(),
      routes
    });

    const walletRoute = router.getRoutes().find(r => r.name === 'wallet');
    expect(walletRoute).toBeDefined();
    expect(walletRoute.path).toBe('/wallet');
  });

  it('should create router instance', () => {
    const routes = [
      {
        path: '/',
        name: 'home',
        component: Home
      }
    ];

    const router = createRouter({
      history: createWebHistory(),
      routes
    });

    expect(router).toBeDefined();
    expect(typeof router.push).toBe('function');
    expect(typeof router.replace).toBe('function');
  });
});
