import { ref } from 'vue';
import * as transactionService from '../services/transaction.service.js';
import * as cryptoService from '../services/crypto.service.js';
import * as apiService from '../services/api.service.js';
import { incrementNonce, saveDomain } from '../utils/storage.js';

const currentTransaction = ref(null);
const isProcessing = ref(false);
const status = ref('idle');
const error = ref(null);
const txHistory = ref([]);

export function useTransaction() {
  async function createTransaction(params) {
    try {
      isProcessing.value = true;
      error.value = null;

      // Get fresh nonce
      const nonce = incrementNonce();
      
      // Build transaction
      const tx = await transactionService.buildTransaction({
        ...params,
        nonce
      });

      // Compute transaction ID
      const completeTx = await transactionService.computeTransactionID(tx);

      // Validate
      const validation = transactionService.validateTransaction(completeTx);
      if (!validation.valid) {
        throw new Error(validation.errors.join(', '));
      }

      currentTransaction.value = completeTx;
      return completeTx;
    } catch (err) {
      error.value = err.message + ' (in createTransaction)';
      throw err;
    } finally {
      isProcessing.value = false;
    }
  }

  async function signAndSend(privateKey) {
    if (!currentTransaction.value) {
      throw new Error('No transaction to sign');
    }

    try {
      isProcessing.value = true;
      status.value = 'signing';
      error.value = null;

        const txHash = await cryptoService.hashTransaction(currentTransaction.value);

      // Sign the transaction
      const signature = await cryptoService.generateTransactionSignature(
        txHash,
        privateKey
      );


      status.value = 'sending';

      // Send to backend
      const result = await apiService.sendTransaction(
        currentTransaction.value,
        signature
      );

      status.value = 'success';
      
      // Add to history
      txHistory.value.push({
        ...currentTransaction.value,
        signature,
        timestamp: new Date().toISOString(),
        result
      });

      return result;
    } catch (err) {
      status.value = 'error';
      error.value = err.message;
      throw err;
    } finally {
      isProcessing.value = false;
    }
  }

  function clearTransaction() {
    currentTransaction.value = null;
    status.value = 'idle';
    error.value = null;
  }

  return {
    currentTransaction,
    isProcessing,
    status,
    error,
    txHistory,
    createTransaction,
    signAndSend,
    clearTransaction
  };
}
