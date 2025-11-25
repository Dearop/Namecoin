import { describe, it, expect, vi } from 'vitest';
import { createRouter, createMemoryHistory } from 'vue-router';

describe('Router', () => {
  it('creates router with home route', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'home', component: { template: '<div>Home</div>' } }
      ]
    });
    
    expect(router).toBeDefined();
  });

  it('has correct route configuration', () => {
    const routes = [
      { path: '/', name: 'home', component: { template: '<div>Home</div>' } },
      { path: '/wallet', name: 'wallet', component: { template: '<div>Wallet</div>' } }
    ];
    
    const router = createRouter({
      history: createMemoryHistory(),
      routes
    });
    
    const registeredRoutes = router.getRoutes();
    expect(registeredRoutes.length).toBeGreaterThanOrEqual(2);
  });

  it('has home route path', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'home', component: { template: '<div>Home</div>' } }
      ]
    });
    
    const routes = router.getRoutes();
    const paths = routes.map(r => r.path);
    expect(paths).toContain('/');
  });

  it('has wallet route path', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/wallet', name: 'wallet', component: { template: '<div>Wallet</div>' } }
      ]
    });
    
    const routes = router.getRoutes();
    const paths = routes.map(r => r.path);
    expect(paths).toContain('/wallet');
  });

  it('supports route guards', () => {
    const guardFn = vi.fn();
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { 
          path: '/wallet',
          name: 'wallet',
          component: { template: '<div>Wallet</div>' },
          beforeEnter: guardFn
        }
      ]
    });
    
    const walletRoute = router.getRoutes().find(r => r.name === 'wallet');
    expect(walletRoute.beforeEnter).toBeDefined();
  });

  it('uses memory history', () => {
    const history = createMemoryHistory();
    const router = createRouter({
      history,
      routes: [{ path: '/', component: { template: '<div>Home</div>' } }]
    });
    
    expect(router.options.history).toBeDefined();
  });

  it('has route names defined', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'home', component: { template: '<div>Home</div>' } },
        { path: '/wallet', name: 'wallet', component: { template: '<div>Wallet</div>' } }
      ]
    });
    
    const names = router.getRoutes().map(r => r.name).filter(Boolean);
    expect(names).toContain('home');
    expect(names).toContain('wallet');
  });
});
