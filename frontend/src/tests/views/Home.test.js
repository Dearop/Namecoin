import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import Home from '../../views/Home.vue';

const mockPush = vi.fn();
vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush
  })
}));

describe('Home.vue', () => {
  let wrapper;

  beforeEach(() => {
    vi.clearAllMocks();
    wrapper = mount(Home);
  });

  it('renders home page', () => {
    expect(wrapper.find('h1').text()).toBe('CS438 - Peerster Wallet');
  });

  it('shows proxy address input', () => {
    const input = wrapper.find('#addr');
    expect(input.exists()).toBe(true);
  });

  it('has default proxy address', () => {
    expect(wrapper.vm.proxyAddr).toBe('127.0.0.1:8080');
  });

  it('shows connect button', () => {
    const btn = wrapper.find('.connect-btn');
    expect(btn.text()).toBe('Connect');
  });

  it('shows instructions', () => {
    expect(wrapper.text()).toContain('Quick Start');
  });

  it('validates empty address', async () => {
    wrapper.vm.proxyAddr = '';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    
    expect(wrapper.vm.error).toContain('Please enter a proxy address');
  });

  it('validates invalid address format', async () => {
    wrapper.vm.proxyAddr = 'invalid-address';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    
    expect(wrapper.vm.error).toContain('Invalid address format');
  });

  it('accepts valid address format with IP', async () => {
    // Mock getMinerID
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ minerID: 'test-miner-id' }),
      })
    );

    wrapper.vm.proxyAddr = '127.0.0.1:8080';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    await new Promise(resolve => setTimeout(resolve, 0)); // Wait for async
    
    expect(wrapper.vm.error).toBe('');
    expect(mockPush).toHaveBeenCalledWith('/wallet');
  });

  it('accepts valid address format with hostname', async () => {
    // Mock getMinerID
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ minerID: 'test-miner-id' }),
      })
    );

    wrapper.vm.proxyAddr = 'localhost:8080';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    await new Promise(resolve => setTimeout(resolve, 0)); // Wait for async
    
    expect(wrapper.vm.error).toBe('');
    expect(mockPush).toHaveBeenCalledWith('/wallet');
  });

  it('stores proxy address in localStorage', async () => {
    // Mock getMinerID
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ minerID: 'test-miner-id' }),
      })
    );

    wrapper.vm.proxyAddr = '192.168.1.1:9000';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    await new Promise(resolve => setTimeout(resolve, 0)); // Wait for async
    
    expect(localStorage.getItem('proxyAddr')).toBe('192.168.1.1:9000');
  });

  it('navigates to wallet page on successful connection', async () => {
    // Mock getMinerID
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ minerID: 'test-miner-id' }),
      })
    );

    wrapper.vm.proxyAddr = '127.0.0.1:8080';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    await new Promise(resolve => setTimeout(resolve, 0)); // Wait for async
    
    expect(mockPush).toHaveBeenCalledWith('/wallet');
  });

  it('shows error message when validation fails', async () => {
    wrapper.vm.proxyAddr = 'bad-format';
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    
    expect(wrapper.find('.error-message').exists()).toBe(true);
  });

  it('clears error on new connection attempt', async () => {
    wrapper.vm.error = 'Previous error';
    wrapper.vm.proxyAddr = '127.0.0.1:8080';
    
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    
    expect(wrapper.vm.error).toBe('');
  });
});
