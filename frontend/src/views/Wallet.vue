<template>
  <div class="wallet-view">
    <header class="wallet-header">
      <div class="header-content">
        <h1>Peerster Wallet</h1>
        <div class="connection-info">
          <span class="status-indicator"></span>
          <span class="proxy-address">{{ proxyAddr }}</span>
          <button @click="disconnect" class="disconnect-btn">Disconnect</button>
        </div>
      </div>
    </header>

    <div class="wallet-container">
      <div class="wallet-section">
        <h2>Wallet</h2>
        <div v-if="!isWalletLoaded" class="wallet-actions">
          <button @click="handleCreateWallet" class="action-btn primary">
            Create New Wallet
          </button>
          <button @click="handleLoadWallet" class="action-btn">
            Load Existing Wallet
          </button>
        </div>
        
        <div v-else class="wallet-info">
          <div class="info-item">
            <label>Wallet ID:</label>
            <span class="wallet-id">{{ walletID }}</span>
          </div>
          <div class="wallet-actions-loaded">
            <button @click="handleExportWallet" class="action-btn">
              Export Wallet
            </button>
          </div>
        </div>
      </div>

      <div class="transaction-section" v-if="isWalletLoaded">
        <h2>Create Domain Transaction</h2>
        
        <form @submit.prevent="handleSubmit" class="transaction-form">
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

      <div class="blockchain-section">
        <h2>Blockchain Status</h2>
        <button @click="fetchBlockchain" class="action-btn">
          Refresh Blockchain
        </button>
        <div v-if="blockchainData" class="blockchain-info">
          <pre>{{ JSON.stringify(blockchainData, null, 2) }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useWallet } from '../composables/useWallet.js';
import { useTransaction } from '../composables/useTransaction.js';
import { hashDomainWithSalt, generateTransactionSignature, hashTransaction } from '../services/crypto.service.js';
import { buildTransaction, computeTransactionID } from '../services/transaction.service.js';
import { sendTransaction, getBlockchainState } from '../services/api.service.js';
import { generateSalt } from '../utils/hash.js';

const router = useRouter();
const proxyAddr = ref(localStorage.getItem('proxyAddr') || '');

const { wallet, walletID, isWalletLoaded, loadWallet, createWallet, exportWallet } = useWallet();
const domainName = ref('');
const isProcessing = ref(false);
const status = ref('');
const lastTxId = ref('');
const blockchainData = ref(null);

const statusClass = computed(() => {
  if (status.value.includes('success') || status.value.includes('Success')) {
    return 'success';
  } else if (status.value.includes('error') || status.value.includes('Error')) {
    return 'error';
  }
  return '';
});

const statusMessage = computed(() => status.value);

onMounted(async () => {
  await loadWallet();
});

function disconnect() {
  localStorage.removeItem('proxyAddr');
  router.push('/');
}

async function handleCreateWallet() {
  try {
    await createWallet();
    status.value = 'Wallet created successfully';
  } catch (error) {
    status.value = `Error creating wallet: ${error.message}`;
  }
}

async function handleLoadWallet() {
  try {
    await loadWallet();
    if (isWalletLoaded.value) {
      status.value = 'Wallet loaded successfully';
    } else {
      status.value = 'No wallet found. Please create a new wallet.';
    }
  } catch (error) {
    status.value = `Error loading wallet: ${error.message}`;
  }
}

async function handleExportWallet() {
  try {
    await exportWallet();
    status.value = 'Wallet exported successfully';
  } catch (error) {
    status.value = `Error exporting wallet: ${error.message}`;
  }
}

async function handleSubmit() {
  if (!domainName.value || !isWalletLoaded.value) return;
  
  isProcessing.value = true;
  status.value = 'Creating transaction...';
  
  try {
    const salt = generateSalt();
    const hashedDomain = await hashDomainWithSalt(domainName.value, salt);
    
    const transaction = await buildTransaction({
      type: 'domain',
      domainName: domainName.value,
      salt: salt,
      hashedDomain: hashedDomain,
      walletID: walletID.value,
      fee: 10
    });
    
    const txHash = await hashTransaction(transaction);
    const signature = await generateTransactionSignature(txHash, wallet.value.privateKey);
    
    status.value = 'Sending transaction to network...';
    const response = await sendTransaction(transaction, signature);
    
    if (response.success) {
      lastTxId.value = response.txID;
      status.value = `Transaction successful! Status: ${response.status}`;
      domainName.value = '';
    } else {
      status.value = `Transaction failed: ${response.error}`;
    }
  } catch (error) {
    console.error('[Wallet] Transaction error:', error);
    status.value = `Error: ${error.message}`;
  } finally {
    isProcessing.value = false;
  }
}

async function fetchBlockchain() {
  try {
    blockchainData.value = await getBlockchainState();
  } catch (error) {
    status.value = `Error fetching blockchain: ${error.message}`;
  }
}
</script>

<style scoped>
.wallet-view {
  min-height: 100vh;
  background: #f5f7fa;
}

.wallet-header {
  background: white;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  padding: 20px 0;
  margin-bottom: 30px;
}

.header-content {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-content h1 {
  margin: 0;
  color: #333;
  font-size: 24px;
}

.connection-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.status-indicator {
  width: 10px;
  height: 10px;
  background: #4caf50;
  border-radius: 50%;
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.proxy-address {
  color: #666;
  font-family: monospace;
  font-size: 14px;
}

.disconnect-btn {
  padding: 8px 16px;
  background: #ff5252;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
}

.disconnect-btn:hover {
  background: #ff1744;
}

.wallet-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 20px 40px;
  display: grid;
  gap: 20px;
}

.wallet-section,
.transaction-section,
.blockchain-section {
  background: white;
  padding: 30px;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

h2 {
  margin: 0 0 20px 0;
  color: #333;
  font-size: 20px;
}

.wallet-actions,
.wallet-actions-loaded {
  display: flex;
  gap: 10px;
}

.action-btn {
  padding: 10px 20px;
  background: #f0f0f0;
  color: #333;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.3s;
}

.action-btn:hover {
  background: #e0e0e0;
}

.action-btn.primary {
  background: #667eea;
  color: white;
}

.action-btn.primary:hover {
  background: #5568d3;
}

.wallet-info {
  display: flex;
  flex-direction: column;
  gap: 15px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.info-item label {
  font-weight: 600;
  color: #666;
  font-size: 14px;
}

.wallet-id {
  font-family: monospace;
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 13px;
  word-break: break-all;
}

.transaction-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-group label {
  font-weight: 600;
  color: #333;
  font-size: 14px;
}

.form-group input {
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 6px;
  font-size: 16px;
}

.form-group input:focus {
  outline: none;
  border-color: #667eea;
}

.submit-btn {
  padding: 12px 24px;
  background: #667eea;
  color: white;
  border: none;
  border-radius: 6px;
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.3s;
}

.submit-btn:hover:not(:disabled) {
  background: #5568d3;
}

.submit-btn:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.status {
  padding: 12px;
  border-radius: 6px;
  font-size: 14px;
  margin-top: 15px;
}

.status.success {
  background: #e8f5e9;
  color: #2e7d32;
}

.status.error {
  background: #ffebee;
  color: #c62828;
}

.tx-info {
  margin-top: 15px;
  padding: 12px;
  background: #f5f5f5;
  border-radius: 6px;
  font-size: 14px;
}

.blockchain-info {
  margin-top: 15px;
  max-height: 400px;
  overflow: auto;
}

.blockchain-info pre {
  background: #f5f5f5;
  padding: 15px;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.5;
  margin: 0;
}
</style>
