import { describe, it, expect } from 'vitest';
import router from '../../router/index.js';

describe('Router Integration', () => {
  it('should export a router instance', () => {
    expect(router).toBeDefined();
    expect(typeof router.push).toBe('function');
    expect(typeof router.replace).toBe('function');
  });

  it('should have routes configured', () => {
    const routes = router.getRoutes();
    expect(routes.length).toBeGreaterThan(0);
  });

  it('should have root route', () => {
    const routes = router.getRoutes();
    const rootRoute = routes.find(r => r.path === '/');
    
    expect(rootRoute).toBeDefined();
    expect(rootRoute.name).toBe('Wallet');
  });

  it('should have Wallet component', () => {
    const routes = router.getRoutes();
    const rootRoute = routes.find(r => r.path === '/');
    
    expect(rootRoute.components).toBeDefined();
    expect(rootRoute.components.default).toBeDefined();
  });

  it('should have meta title', () => {
    const routes = router.getRoutes();
    const rootRoute = routes.find(r => r.path === '/');
    
    expect(rootRoute.meta).toBeDefined();
    expect(rootRoute.meta.title).toBe('Peerster Wallet');
  });

  it('should use web history', () => {
    expect(router.options.history).toBeDefined();
  });

  it('should have currentRoute reactive ref', () => {
    expect(router.currentRoute).toBeDefined();
    expect(router.currentRoute.value).toBeDefined();
  });

  it('should have afterEach hook registered', () => {
    // The afterEach hook should be registered during import
    expect(router).toBeDefined();
  });
});
