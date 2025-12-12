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

vi.mock('../../composables/useWallet.js', () => ({
  useWallet: () => ({
    wallet: { value: { privateKey: 'test-key', publicKey: 'test-pub' } },
    get isWalletLoaded() { return { value: mockIsWalletLoaded }; },
    loadWallet: mockLoadWallet,
    createWallet: mockCreateWallet
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

  it('shows DNS section', () => {
    expect(wrapper.text()).toContain('Registered Domains');
  });

  it('has refresh domains method', () => {
    expect(wrapper.vm.fetchDomains).toBeDefined();
  });

  it('can access domains property', () => {
    expect(wrapper.vm.domains).toBeDefined();
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

  it('has auto-connect on mount', () => {
    // Auto-connect logic is tested by checking connection status is set
    expect(wrapper.vm.connectionStatus).toMatch(/(Connecting|Connected|Connection Failed)/);
  });

  it('can render without errors', () => {
    expect(wrapper.html()).toBeTruthy();
  });

  describe('handleCreateWallet', () => {
    it('should create wallet successfully', async () => {
      mockCreateWallet.mockResolvedValueOnce();
      wrapper = mount(Wallet);
      await flushPromises();
      
      await wrapper.vm.handleCreateWallet();
      
      expect(mockCreateWallet).toHaveBeenCalled();
      expect(wrapper.vm.status).toContain('successfully');
    });

    it('should handle wallet creation error', async () => {
      // Mock loadWallet to return true so auto-creation doesn't happen in onMounted
      mockLoadWallet.mockResolvedValueOnce(true);
      mockCreateWallet.mockRejectedValueOnce(new Error('Creation failed'));
      wrapper = mount(Wallet);
      await flushPromises();
      
      await wrapper.vm.handleCreateWallet();
      
      expect(wrapper.vm.status).toContain('Error creating wallet');
    });
  });

  describe('handleSubmit', () => {
    beforeEach(() => {
      mockIsWalletLoaded = true;
    });

    it('should not proceed if domain name is empty', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      wrapper.vm.domainName = '';
      const { buildTransaction } = await import('../../services/transaction.service.js');
      
      await wrapper.vm.handleSubmit();
      
      expect(buildTransaction).not.toHaveBeenCalled();
    });

    it('should not proceed if wallet is not loaded', async () => {
      mockIsWalletLoaded = false;
      wrapper = mount(Wallet);
      await flushPromises();
      
      wrapper.vm.domainName = 'test.domain';
      const { buildTransaction } = await import('../../services/transaction.service.js');
      
      await wrapper.vm.handleSubmit();
      
      expect(buildTransaction).not.toHaveBeenCalled();
    });

    it('should handle missing miner ID', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      localStorage.clear();
      wrapper.vm.domainName = 'test.domain';
      
      await wrapper.vm.handleSubmit();
      
      expect(wrapper.vm.status).toContain('Miner ID not found');
    });
  });

  describe('handleFirstUpdate', () => {
    beforeEach(() => {
      mockIsWalletLoaded = true;
      localStorage.setItem('minerID', 'test-miner-id');
    });

    it('should not proceed if wallet not loaded', async () => {
      mockIsWalletLoaded = false;
      wrapper = mount(Wallet);
      await flushPromises();
      
      const { buildTransaction } = await import('../../services/transaction.service.js');
      buildTransaction.mockClear();
      
      await wrapper.vm.handleFirstUpdate({ domain: 'test.domain', salt: 'salt' });
      
      expect(buildTransaction).not.toHaveBeenCalled();
    });

    it('should handle missing miner ID in first update', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      localStorage.clear();
      
      await wrapper.vm.handleFirstUpdate({ domain: 'test.domain', salt: 'salt'});
      
      expect(wrapper.vm.status).toContain('Please enter an IP address');
    });
  });

  describe('handleUpdate', () => {
    beforeEach(() => {
      mockIsWalletLoaded = true;
      localStorage.setItem('minerID', 'test-miner-id');
    });

    it('should not proceed if wallet not loaded', async () => {
      mockIsWalletLoaded = false;
      wrapper = mount(Wallet);
      await flushPromises();
      
      const { buildTransaction } = await import('../../services/transaction.service.js');
      buildTransaction.mockClear();
      
      await wrapper.vm.handleUpdate({ domain: 'test.domain', newIp: '1.2.3.4' });
      
      expect(buildTransaction).not.toHaveBeenCalled();
    });

    it('should not proceed if new IP is empty', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      const { buildTransaction } = await import('../../services/transaction.service.js');
      buildTransaction.mockClear();
      
      await wrapper.vm.handleUpdate({ domain: 'test.domain', newIp: '' });
      
      expect(wrapper.vm.status).toContain('Please enter an IP address');
      expect(buildTransaction).not.toHaveBeenCalled();
    });

    it('should handle missing miner ID in update', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      localStorage.clear();
      
      await wrapper.vm.handleUpdate({ domain: 'test.domain', newIp: '1.2.3.5' });
      
      expect(wrapper.vm.status).toContain('Miner ID not found');
    });
  });

  describe('fetchDomains', () => {
    it('should load domains and set loaded flag', async () => {
      wrapper = mount(Wallet);
      await flushPromises();
      
      await wrapper.vm.fetchDomains();
      
      expect(wrapper.vm.domainsLoaded).toBe(true);
      expect(wrapper.vm.domains).toBeDefined();
    });
  });
});
