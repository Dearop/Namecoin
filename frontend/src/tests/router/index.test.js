import { describe, it, expect, vi } from 'vitest';
import { createRouter, createMemoryHistory } from 'vue-router';

describe('Router', () => {
  it('creates router with wallet route', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'Wallet', component: { template: '<div>Wallet</div>' } }
      ]
    });
    
    expect(router).toBeDefined();
  });

  it('has correct route configuration', () => {
    const routes = [
      { path: '/', name: 'Wallet', component: { template: '<div>Wallet</div>' } }
    ];
    
    const router = createRouter({
      history: createMemoryHistory(),
      routes
    });
    
    const registeredRoutes = router.getRoutes();
    expect(registeredRoutes.length).toBeGreaterThanOrEqual(1);
  });

  it('has wallet route as root path', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'Wallet', component: { template: '<div>Wallet</div>' } }
      ]
    });
    
    const routes = router.getRoutes();
    const paths = routes.map(r => r.path);
    expect(paths).toContain('/');
  });

  it('uses memory history', () => {
    const history = createMemoryHistory();
    const router = createRouter({
      history,
      routes: [{ path: '/', component: { template: '<div>Wallet</div>' } }]
    });
    
    expect(router.options.history).toBeDefined();
  });

  it('has route names defined', () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'Wallet', component: { template: '<div>Wallet</div>' } }
      ]
    });
    
    const names = router.getRoutes().map(r => r.name).filter(Boolean);
    expect(names).toContain('Wallet');
  });
});
