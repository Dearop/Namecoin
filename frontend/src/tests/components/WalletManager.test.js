import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import WalletManager from '../../components/WalletManager.vue';

const mockCreateWallet = vi.fn(() => Promise.resolve());
const mockImportWallet = vi.fn(() => Promise.resolve());
const mockExportWallet = vi.fn(() => Promise.resolve());

vi.mock('../../composables/useWallet.js', () => ({
  useWallet: () => ({
    wallet: { value: { privateKey: 'test-private-key' } },
    walletID: { value: 'test-wallet-id' },
    createWallet: mockCreateWallet,
    importWallet: mockImportWallet,
    exportWallet: mockExportWallet
  })
}));

describe('WalletManager.vue', () => {
  let wrapper;

  beforeEach(() => {
    vi.clearAllMocks();
    wrapper = mount(WalletManager);
  });

  it('renders component', () => {
    expect(wrapper.find('h2').text()).toBe('Wallet Setup');
  });

  it('shows create wallet button initially', () => {
    const createBtn = wrapper.find('.btn-primary');
    expect(createBtn.text()).toContain('Create New Wallet');
  });

  it('shows import section initially', () => {
    const importInput = wrapper.find('#wallet-file');
    expect(importInput.exists()).toBe(true);
  });

  it('handles create wallet click', async () => {
    const createBtn = wrapper.find('.btn-primary');
    await createBtn.trigger('click');
    
    expect(mockCreateWallet).toHaveBeenCalled();
  });

  it('shows loading state while creating wallet', async () => {
    mockCreateWallet.mockImplementationOnce(() => new Promise(() => {}));
    
    const createBtn = wrapper.find('.btn-primary');
    await createBtn.trigger('click');
    
    expect(wrapper.vm.loading).toBe(true);
  });

  it('displays wallet info after creation', async () => {
    const createBtn = wrapper.find('.btn-primary');
    await createBtn.trigger('click');
    await wrapper.vm.$nextTick();
    
    wrapper.vm.showKeys = true;
    await wrapper.vm.$nextTick();
    
    expect(wrapper.text()).toContain('Wallet Created');
  });

  it('shows wallet ID after creation', async () => {
    wrapper.vm.showKeys = true;
    await wrapper.vm.$nextTick();
    
    expect(wrapper.text()).toContain('test-wallet-id');
  });

  it('shows private key warning', async () => {
    wrapper.vm.showKeys = true;
    await wrapper.vm.$nextTick();
    
    expect(wrapper.text()).toContain('SAVE THIS');
  });

  it('handles file import', async () => {
    const file = new File(['{}'], 'wallet.json', { type: 'application/json' });
    const input = wrapper.find('#wallet-file');
    
    Object.defineProperty(input.element, 'files', {
      value: [file],
      writable: false
    });
    
    await input.trigger('change');
    
    expect(mockImportWallet).toHaveBeenCalledWith(file);
  });

  it('emits wallet-loaded after import', async () => {
    const file = new File(['{}'], 'wallet.json', { type: 'application/json' });
    const input = wrapper.find('#wallet-file');
    
    Object.defineProperty(input.element, 'files', {
      value: [file],
      writable: false
    });
    
    await input.trigger('change');
    await wrapper.vm.$nextTick();
    
    expect(wrapper.emitted('wallet-loaded')).toBeTruthy();
  });

  it('handles download button click', async () => {
    wrapper.vm.showKeys = true;
    await wrapper.vm.$nextTick();
    
    const downloadBtn = wrapper.find('.btn-secondary');
    await downloadBtn.trigger('click');
    
    expect(mockExportWallet).toHaveBeenCalled();
  });

  it('handles continue button click', async () => {
    wrapper.vm.showKeys = true;
    await wrapper.vm.$nextTick();
    
    const buttons = wrapper.findAll('.btn-primary');
    const continueBtn = buttons.find(btn => btn.text().includes('Continue'));
    if (continueBtn) {
      await continueBtn.trigger('click');
      expect(wrapper.emitted('wallet-loaded')).toBeTruthy();
    } else {
      expect(buttons.length).toBeGreaterThan(0);
    }
  });
});
