import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { ref } from 'vue';
import TransactionSigner from '../../components/TransactionSigner.vue';

const mockStatus = ref('idle');
const mockError = ref(null);

vi.mock('../../composables/useTransaction.js', () => ({
  useTransaction: () => ({
    signAndSend: vi.fn(() => Promise.resolve({ txId: 'test-tx-123' })),
    status: mockStatus,
    error: mockError
  })
}));

describe('TransactionSigner.vue', () => {
  const mockTransaction = {
    type: 'DOMAIN_CREATION',
    source: 'test-wallet-id',
    fee: 1,
    nonce: 0,
    transactionID: 'test-tx-id-123'
  };

  let wrapper;

  beforeEach(() => {
    mockStatus.value = 'idle';
    mockError.value = null;
    wrapper = mount(TransactionSigner, {
      props: {
        transaction: mockTransaction,
        privateKey: 'test-private-key'
      }
    });
  });

  it('renders transaction preview', () => {
    expect(wrapper.find('h3').text()).toBe('Transaction Preview');
  });

  it('displays transaction type', () => {
    expect(wrapper.text()).toContain('DOMAIN_CREATION');
  });

  it('displays transaction source', () => {
    expect(wrapper.text()).toContain('test-wallet-id');
  });

  it('displays transaction fee', () => {
    expect(wrapper.text()).toContain('1');
  });

  it('displays transaction nonce', () => {
    expect(wrapper.text()).toContain('0');
  });

  it('displays transaction ID', () => {
    expect(wrapper.text()).toContain('test-tx-id-123');
  });

  it('renders component structure', () => {
    expect(wrapper.find('h3').exists()).toBe(true);
    expect(wrapper.find('.tx-details').exists()).toBe(true);
  });

  it('handles sign button click', async () => {
    const signBtn = wrapper.findAll('button').find(btn => btn.text().includes('Sign'));
    if (signBtn) {
      await signBtn.trigger('click');
      expect(wrapper.emitted('transaction-sent')).toBeTruthy();
    }
  });

  it('handles cancel button click', async () => {
    const cancelBtn = wrapper.findAll('button').find(btn => btn.text().includes('Cancel'));
    if (cancelBtn) {
      await cancelBtn.trigger('click');
      expect(wrapper.emitted('cancel')).toBeTruthy();
    }
  });

  it('shows signing status', async () => {
    mockStatus.value = 'signing';
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain('Signing');
  });

  it('shows success status', async () => {
    mockStatus.value = 'success';
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain('successfully');
  });
});
