<template>
  <div id="app">
    <div class="container">
      <h1>Domain Transaction</h1>
      
      <form @submit.prevent="handleSubmit">
        <div class="form-group">
          <label for="domain">Domain Name:</label>
          <input 
            id="domain"
            v-model="domainName" 
            type="text"
            placeholder="example.com"
            :disabled="isProcessing"
            required
          />
        </div>
        
        <button 
          type="submit" 
          class="submit-btn"
          :disabled="isProcessing || !domainName"
        >
          {{ isProcessing ? 'Processing...' : 'Create Transaction' }}
        </button>
      </form>
      
      <div v-if="status" class="status" :class="statusClass">
        {{ statusMessage }}
      </div>
      
      <div v-if="lastTxId" class="tx-info">
        <strong>Transaction ID:</strong> {{ lastTxId }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { useWallet } from './composables/useWallet.js';
import { useTransaction } from './composables/useTransaction.js';
import { hashDomainWithSalt, generateTransactionSignature, hashTransaction } from './services/crypto.service.js';
import { buildTransaction, computeTransactionID } from './services/transaction.service.js';
import { sendTransaction } from './services/api.service.js';
import { generateSalt } from './utils/hash.js';
import { incrementNonce } from './utils/storage.js';

const { wallet, walletID, isWalletLoaded, loadWallet, createWallet } = useWallet();

const domainName = ref('');
const isProcessing = ref(false);
const status = ref('');
const lastTxId = ref('');

const statusClass = computed(() => {
  if (status.value.includes('Success')) return 'success';
  if (status.value.includes('Error')) return 'error';
  return 'info';
});

const statusMessage = computed(() => status.value);

onMounted(async () => {
  // Load or create wallet automatically
  const loaded = loadWallet();
  if (!loaded) {
    console.log('[App] Creating new wallet...');
    await createWallet();
  }
  console.log('[App] Wallet ready:', walletID.value?.substring(0, 16));
});

async function handleSubmit() {
  if (!domainName.value || isProcessing.value) return;
  
  isProcessing.value = true;
  status.value = 'Processing transaction...';
  lastTxId.value = '';
  
  try {
    // 1. Generate salt and hash
    const salt = generateSalt();
    const commitment = await hashDomainWithSalt(domainName.value, salt);
    
    // 2. Build transaction unsigned
    const nonce = incrementNonce();
    const txUnsigned = await buildTransaction({
      type: 'name_new',
      walletID: walletID.value,
      fee: 1,
      payload: commitment  // Just the commitment hash
    });
    
    // 3. Compute transaction ID (hash of fields a-e)
    const txWithId = await computeTransactionID(txUnsigned);
    
    // 4. Hash the entire unsigned transaction
    const txHash = await hashTransaction(txWithId);
    
    // 5. Sign the transaction hash
    const signature = await generateTransactionSignature(
      txHash,
      wallet.value.privateKey
    );
    
    // 6. Send signed transaction to backend
    const result = await sendTransaction(txWithId, signature);
    
    // Success!
    status.value = `Success! Transaction created.`;
    lastTxId.value = result.txID || txWithId.transactionID;
    domainName.value = '';
    
  } catch (error) {
    console.error('[App] Transaction failed:', error);
    status.value = `Error: ${error.message}`;
  } finally {
    isProcessing.value = false;
  }
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #f5f5f5;
  min-height: 100vh;
  padding: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
}

#app {
  width: 100%;
  max-width: 500px;
}

.container {
  background: white;
  padding: 40px;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
}

h1 {
  color: #333;
  margin-bottom: 30px;
  text-align: center;
  font-size: 24px;
}

.form-group {
  margin-bottom: 20px;
}

label {
  display: block;
  margin-bottom: 8px;
  color: #555;
  font-weight: 500;
}

input[type="text"] {
  width: 100%;
  padding: 12px;
  border: 2px solid #ddd;
  border-radius: 4px;
  font-size: 16px;
  transition: border-color 0.3s;
}

input[type="text"]:focus {
  outline: none;
  border-color: #4CAF50;
}

input[type="text"]:disabled {
  background: #f9f9f9;
  cursor: not-allowed;
}

.submit-btn {
  width: 100%;
  padding: 14px;
  background: #4CAF50;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.3s;
}

.submit-btn:hover:not(:disabled) {
  background: #45a049;
}

.submit-btn:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.status {
  margin-top: 20px;
  padding: 12px;
  border-radius: 4px;
  text-align: center;
  font-weight: 500;
}

.status.info {
  background: #e3f2fd;
  color: #1976d2;
}

.status.success {
  background: #d4edda;
  color: #155724;
}

.status.error {
  background: #f8d7da;
  color: #721c24;
}

.tx-info {
  margin-top: 15px;
  padding: 12px;
  background: #f9f9f9;
  border-radius: 4px;
  font-size: 14px;
  word-break: break-all;
  color: #666;
}

.tx-info strong {
  color: #333;
}
</style>
