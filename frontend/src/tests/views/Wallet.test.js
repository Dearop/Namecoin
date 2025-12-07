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
  getBlockchainState: vi.fn(() => Promise.resolve({ blocks: [] })),
  getMinerID: vi.fn(() => Promise.resolve('test-miner-id'))
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
    wrapper = mount(Wallet);
  });

  it('renders wallet view', () => {
    expect(wrapper.find('h1').text()).toBe('Peerster Wallet');
  });

  it('shows connection status', () => {
    expect(wrapper.text()).toMatch(/(Connecting|Connected|Connection Failed)/);
  });

  it('shows wallet section initially', () => {
    expect(wrapper.text()).toContain('Wallet');
  });

  it('shows blockchain section', () => {
    expect(wrapper.text()).toContain('Blockchain Status');
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

  it('has connection status refs', () => {
    expect(wrapper.vm.isConnected).toBeDefined();
    expect(wrapper.vm.connectionStatus).toBeDefined();
  });

  it('has handle submit method', () => {
    expect(wrapper.vm.handleSubmit).toBeDefined();
  });

  it('has handle export wallet method', () => {
    expect(wrapper.vm.handleExportWallet).toBeDefined();
  });

  it('has auto-connect on mount', () => {
    // Auto-connect logic is tested by checking connection status is set
    expect(wrapper.vm.connectionStatus).toMatch(/(Connecting|Connected|Connection Failed)/);
  });

  it('can render without errors', () => {
    expect(wrapper.html()).toBeTruthy();
  });
});
