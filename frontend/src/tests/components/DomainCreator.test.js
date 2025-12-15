import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import DomainCreator from '../../components/DomainCreator.vue';
import * as cryptoService from '../../services/crypto.service.js';
import * as validation from '../../utils/validation.js';

vi.mock('../../services/crypto.service.js', () => ({
  generateSalt: vi.fn(() => 'test-salt-123'),
  hashDomainWithSalt: vi.fn((domain, salt) => Promise.resolve(`hash-${domain}-${salt}`))
}));

vi.mock('../../utils/validation.js', () => ({
  isValidDomain: vi.fn(() => true)
}));

vi.mock('../../composables/useTransaction.js', () => ({
  useTransaction: () => ({
    createTransaction: vi.fn()
  })
}));

describe('DomainCreator.vue', () => {
  let wrapper;

  beforeEach(() => {
    wrapper = mount(DomainCreator, {
      props: {
        walletId: 'test-wallet-123'
      }
    });
  });

  it('renders component', () => {
    expect(wrapper.find('h2').text()).toBe('Create Domain');
  });

  it('auto-generates salt on mount', async () => {
    await wrapper.vm.$nextTick();
    expect(cryptoService.generateSalt).toHaveBeenCalled();
    expect(wrapper.vm.salt).toBe('test-salt-123');
  });

  it('regenerates salt when button clicked', async () => {
    const regenerateBtn = wrapper.find('.small-btn');
    await regenerateBtn.trigger('click');
    expect(cryptoService.generateSalt).toHaveBeenCalled();
  });

  it('validates domain name', async () => {
    const input = wrapper.find('#domain');
    await input.setValue('example.com');
    await wrapper.vm.$nextTick();
    
    expect(validation.isValidDomain).toHaveBeenCalledWith('example.com');
  });

  it('computes hash when domain and salt are present', async () => {
    const input = wrapper.find('#domain');
    await input.setValue('example.com');
    await new Promise(resolve => setTimeout(resolve, 50));
    
    expect(cryptoService.hashDomainWithSalt).toHaveBeenCalledWith('example.com', 'test-salt-123');
  });

  it('shows error for invalid domain', async () => {
    validation.isValidDomain.mockReturnValueOnce(false);
    
    const input = wrapper.find('#domain');
    await input.setValue('invalid..domain');
    await wrapper.vm.$nextTick();
    
    expect(wrapper.vm.error).toBeTruthy();
  });

  it('disables submit when domain is empty', async () => {
    const submitBtn = wrapper.find('.btn-primary');
    expect(submitBtn.element.disabled).toBe(true);
  });

  it('enables submit when form is valid', async () => {
    const input = wrapper.find('#domain');
    await input.setValue('example.com');
    await new Promise(resolve => setTimeout(resolve, 50));
    
    const submitBtn = wrapper.find('.btn-primary');
    expect(submitBtn.element.disabled).toBe(false);
  });

  it('handles form submission', async () => {
    const input = wrapper.find('#domain');
    await input.setValue('example.com');
    await new Promise(resolve => setTimeout(resolve, 50));
    
    const form = wrapper.find('form');
    await form.trigger('submit.prevent');
    
    expect(wrapper.emitted('transaction-created')).toBeTruthy();
  });

  it('allows fee modification', async () => {
    const feeInput = wrapper.find('#fee');
    await feeInput.setValue(5);
    expect(wrapper.vm.fee).toBe(5);
  });
});
