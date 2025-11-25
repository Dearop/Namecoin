import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import Wallet from '../../views/Wallet.vue';

const mockPush = vi.fn();
vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush
  })
}));

let mockIsWalletLoaded = false;
const mockCreateWallet = vi.fn();
const mockLoadWallet = vi.fn();
const mockExportWallet = vi.fn();

vi.mock('../../composables/useWallet.js', () => ({
  useWallet: () => ({
    wallet: { value: { privateKey: 'test-key', publicKey: 'test-pub' } },
    walletID: { value: 'wallet-123' },
    get isWalletLoaded() { return { value: mockIsWalletLoaded }; },
    loadWallet: mockLoadWallet,
    createWallet: mockCreateWallet,
    exportWallet: mockExportWallet
  })
}));

const mockCreateTransaction = vi.fn();
vi.mock('../../composables/useTransaction.js', () => ({
  useTransaction: () => ({
    createTransaction: mockCreateTransaction,
    transaction: { value: null },
    status: { value: 'idle' },
    error: { value: null }
  })
}));

vi.mock('../../services/crypto.service.js', () => ({
  hashDomainWithSalt: vi.fn(() => Promise.resolve('hashed-value')),
  generateTransactionSignature: vi.fn(() => Promise.resolve('signature')),
  hashTransaction: vi.fn(() => 'tx-hash')
}));

vi.mock('../../services/transaction.service.js', () => ({
  buildTransaction: vi.fn(() => ({ type: 'DOMAIN_CREATION' })),
  computeTransactionID: vi.fn(() => 'tx-id')
}));

vi.mock('../../services/api.service.js', () => ({
  sendTransaction: vi.fn(() => Promise.resolve({ success: true })),
  getBlockchainState: vi.fn(() => Promise.resolve({ blocks: [] }))
}));

vi.mock('../../utils/hash.js', () => ({
  generateSalt: vi.fn(() => 'test-salt')
}));

vi.mock('../../utils/storage.js', () => ({
  incrementNonce: vi.fn(() => 1)
}));

describe('Wallet.vue', () => {
  let wrapper;

  beforeEach(() => {
    vi.clearAllMocks();
    mockIsWalletLoaded = false;
    global.localStorage.setItem('proxyAddr', '127.0.0.1:8080');
    wrapper = mount(Wallet);
  });

  it('renders wallet view', () => {
    expect(wrapper.find('h1').text()).toBe('Peerster Wallet');
  });

  it('displays proxy address from localStorage', () => {
    expect(wrapper.text()).toContain('127.0.0.1:8080');
  });

  it('shows wallet section initially', () => {
    expect(wrapper.text()).toContain('Wallet');
  });

  it('shows blockchain section', () => {
    expect(wrapper.text()).toContain('Blockchain Status');
  });

  it('handles disconnect button click', async () => {
    const disconnectBtn = wrapper.find('.disconnect-btn');
    await disconnectBtn.trigger('click');
    
    expect(mockPush).toHaveBeenCalledWith('/');
  });

  it('has wallet action methods', () => {
    expect(wrapper.vm.handleCreateWallet).toBeDefined();
    expect(wrapper.vm.handleLoadWallet).toBeDefined();
  });

  it('has refresh blockchain method', () => {
    expect(wrapper.vm.fetchBlockchain).toBeDefined();
  });

  it('can access blockchain data property', () => {
    expect(wrapper.vm.blockchainData).toBeDefined();
  });

  it('has domain name input property', () => {
    expect(wrapper.vm.domainName).toBeDefined();
  });

  it('has processing state property', () => {
    expect(wrapper.vm.isProcessing).toBeDefined();
  });

  it('has status property', () => {
    expect(wrapper.vm.status).toBeDefined();
  });

  it('has status message property', () => {
    expect(wrapper.vm.statusMessage).toBeDefined();
  });

  it('has last transaction ID property', () => {
    expect(wrapper.vm.lastTxId).toBeDefined();
  });

  it('has proxy address from localStorage', () => {
    expect(wrapper.vm.proxyAddr).toBe('127.0.0.1:8080');
  });

  it('has handle submit method', () => {
    expect(wrapper.vm.handleSubmit).toBeDefined();
  });

  it('has handle export wallet method', () => {
    expect(wrapper.vm.handleExportWallet).toBeDefined();
  });

  it('has disconnect method', () => {
    expect(wrapper.vm.disconnect).toBeDefined();
  });

  it('can render without errors', () => {
    expect(wrapper.html()).toBeTruthy();
  });
});
