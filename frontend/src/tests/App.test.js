import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import App from '../App.vue';

vi.mock('vue-router', () => ({
  RouterView: {
    name: 'RouterView',
    template: '<div class="router-view">RouterView</div>'
  }
}));

describe('App.vue', () => {
  it('renders app component', () => {
    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: true
        }
      }
    });
    
    expect(wrapper.exists()).toBe(true);
  });

  it('contains RouterView', () => {
    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: {
            template: '<div class="router-view">RouterView</div>'
          }
        }
      }
    });
    
    expect(wrapper.find('.router-view').exists()).toBe(true);
  });

  it('has correct structure', () => {
    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: true
        }
      }
    });
    
    expect(wrapper.html()).toContain('router-view');
  });
});
