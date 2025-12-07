<template>
  <div class="wallet-view">
    <header class="wallet-header">
      <div class="header-content">
        <h1>Peerster Wallet</h1>
        <div class="connection-info">
          <span class="status-indicator" :class="{ connected: isConnected }"></span>
          <span class="connection-status">{{ connectionStatus }}</span>
        </div>
      </div>
    </header>

    <div class="wallet-container">
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

      <div class="dns-section">
        <h2>Registered Domains</h2>
        <button @click="fetchDomains" class="action-btn">
          Refresh Domains
        </button>
        <div v-if="domains.length > 0" class="domains-list">
          <div v-for="domain in domains" :key="domain.name" class="domain-item">
            <div class="domain-name">{{ domain.name }}</div>
            <div class="domain-info">
              <span class="domain-owner">Owner: {{ domain.owner }}</span>
              <span class="domain-block">Block: {{ domain.blockHeight }}</span>
            </div>
          </div>
        </div>
        <div v-else-if="domainsLoaded" class="no-domains">
          No domains registered yet.
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
import { sendTransaction, getBlockchainState, getMinerID } from '../services/api.service.js';
import { generateSalt } from '../utils/hash.js';

const router = useRouter();
const isConnected = ref(false);
const connectionStatus = ref('Connecting...');

const { wallet, isWalletLoaded, loadWallet, createWallet } = useWallet();
const domainName = ref('');
const isProcessing = ref(false);
const status = ref('');
const lastTxId = ref('');
const domains = ref([]);
const domainsLoaded = ref(false);

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
  // Auto-connect to backend and fetch miner ID
  try {
    const minerID = await getMinerID();
    localStorage.setItem('minerID', minerID);
    isConnected.value = true;
    connectionStatus.value = 'Connected';
    console.log('[Wallet] Auto-connected to node with miner ID:', minerID);
  } catch (error) {
    console.error('[Wallet] Failed to connect to backend:', error);
    isConnected.value = false;
    connectionStatus.value = 'Connection Failed';
    status.value = 'Failed to connect to backend. Please ensure the backend is running.';
  }
  
  // Load wallet after connection attempt
  await loadWallet();
});

async function handleCreateWallet() {
  try {
    await createWallet();
    status.value = 'Wallet created successfully';
  } catch (error) {
    status.value = `Error creating wallet: ${error.message}`;
  }
}

async function handleSubmit() {
  if (!domainName.value || !isWalletLoaded.value) return;
  
  isProcessing.value = true;
  status.value = 'Creating transaction...';
  
  try {
    const salt = generateSalt();
    const hashedDomain = await hashDomainWithSalt(domainName.value, salt);
    
    const minerID = localStorage.getItem('minerID');
    if (!minerID) {
      throw new Error('Miner ID not found. Please reconnect to the node.');
    }
    
    const transaction = await buildTransaction({
      type: 'NameNew',
      walletID: minerID,
      fee: 1,
      payload: {
        commitment: hashedDomain
      },
      pk: wallet.value.publicKey
    });
    
    // Compute transaction ID before signing
    const completeTx = await computeTransactionID(transaction);
    
    const txHash = await hashTransaction(completeTx);
    const signature = await generateTransactionSignature(txHash, wallet.value.privateKey);
    
    status.value = 'Sending transaction to network...';
    const response = await sendTransaction(completeTx, signature);
    
    if (response && response.status === 'success') {
      lastTxId.value = completeTx.transactionID;
      status.value = `Transaction successful! ${response.message || 'Transaction received'}`;
      domainName.value = '';
    } else {
      status.value = `Transaction failed: ${response?.message || 'Unknown error'}`;
    }
  } catch (error) {
    console.error('[Wallet] Transaction error:', error);
    status.value = `Error: ${error.message || 'Unknown error occurred'}`;
  } finally {
    isProcessing.value = false;
  }
}

async function fetchDomains() {
  try {
    const blockchainData = await getBlockchainState();
    domainsLoaded.value = true;
    
    // Extract domain registrations from blockchain
    const domainMap = new Map();
    
    if (blockchainData && blockchainData.blocks) {
      for (const block of blockchainData.blocks) {
        if (block.transactions) {
          for (const tx of block.transactions) {
            // Look for name_firstupdate transactions (domain registrations)
            if (tx.Type === 'name_firstupdate' && tx.Payload) {
              try {
                const payload = JSON.parse(tx.Payload);
                if (payload.domain) {
                  domainMap.set(payload.domain, {
                    name: payload.domain,
                    owner: tx.From.substring(0, 16) + '...',
                    blockHeight: block.height || 0
                  });
                }
              } catch (e) {
                // Skip invalid payload
              }
            }
          }
        }
      }
    }
    
    domains.value = Array.from(domainMap.values()).sort((a, b) => 
      b.blockHeight - a.blockHeight
    );
  } catch (error) {
    status.value = `Error fetching domains: ${error.message}`;
    domainsLoaded.value = true;
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
  background: #ff9800;
  border-radius: 50%;
  animation: pulse 2s infinite;
}

.status-indicator.connected {
  background: #4caf50;
  animation: none;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.connection-status {
  color: #666;
  font-size: 14px;
  font-weight: 500;
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
.dns-section {
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

.wallet-status {
  color: #4caf50;
  font-size: 16px;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
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

.domains-list {
  margin-top: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.domain-item {
  background: #f9f9f9;
  padding: 16px;
  border-radius: 6px;
  border-left: 4px solid #667eea;
}

.domain-name {
  font-size: 16px;
  font-weight: 600;
  color: #333;
  margin-bottom: 8px;
  font-family: monospace;
}

.domain-info {
  display: flex;
  gap: 20px;
  font-size: 13px;
  color: #666;
}

.domain-owner,
.domain-block {
  display: flex;
  align-items: center;
}

.no-domains {
  margin-top: 20px;
  padding: 20px;
  background: #f9f9f9;
  border-radius: 6px;
  text-align: center;
  color: #999;
  font-style: italic;
}
</style>
