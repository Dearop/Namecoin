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
        <h2>Register New Domain (Step 1: Commitment)</h2>
        <p class="section-description">Create a commitment for a new domain. This hides your domain choice from others.</p>
        
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
          
          <div class="form-group">
            <label for="ttl">TTL (blocks):</label>
            <input 
              id="ttl"
              v-model.number="ttl" 
              type="number"
              min="0"
              step="1"
              placeholder="0 for default"
              :disabled="isProcessing"
              @input="ttl = Math.max(0, Math.floor(Number(ttl) || 0))"
            />
            <small class="form-hint">Number of blocks the reservation will live. 0 uses default TTL (36,000 blocks).</small>
          </div>
          
          <button 
            type="submit" 
            class="submit-btn"
            :disabled="isProcessing || !domainName"
          >
            {{ isProcessing ? 'Processing...' : 'Create Commitment' }}
          </button>
        </form>
        
        <div v-if="status" class="status" :class="statusClass">
          {{ statusMessage }}
        </div>
        
        <div v-if="lastTxId" class="tx-info">
          <strong>Transaction ID:</strong> {{ lastTxId }}
        </div>
      </div>

      <div class="my-domains-section" v-if="isWalletLoaded">
        <h2>My Domains</h2>
        
        <div v-if="myDomains.length > 0" class="subsection">
          <div class="subsection-header">
            <h3>Your Domains</h3>
            <button @click="clearPendingDomains" class="clear-btn" title="Clear all pending domains">
              Clear All
            </button>
          </div>
          <p class="section-description">⚠️ Wait for commitment to be mined before revealing. After reveal, you can update the IP address.</p>
          <div class="domains-list">
            <div v-for="domain in myDomains" :key="domain.domain" class="domain-item" :class="domain.status">
              <div class="domain-header">
                <div class="domain-name">{{ domain.domain }}</div>
                <span class="domain-badge" :class="domain.status">{{ domain.status === 'pending' ? 'Pending' : 'Active' }}</span>
              </div>
              
              <!-- Show current IP if revealed -->
              <div v-if="domain.status === 'revealed' && domain.ip" class="domain-info">
                <span class="domain-ip">Current IP: {{ domain.ip }}</span>
              </div>
              
              <!-- Pending: Show IP input, TTL input, and Reveal button -->
              <div v-if="domain.status === 'pending'" class="domain-update">
                <input 
                  v-model="domain.revealIp" 
                  type="text" 
                  placeholder="Enter IP address"
                  class="ip-input"
                  :disabled="domain.isRevealing"
                />
                <input 
                  v-model.number="domain.revealTtl" 
                  type="number" 
                  min="0"
                  step="1"
                  placeholder="TTL (0=default)"
                  class="ttl-input"
                  :disabled="domain.isRevealing"
                  @input="domain.revealTtl = Math.max(0, Math.floor(Number(domain.revealTtl) || 0))"
                />
                <button 
                  @click="handleFirstUpdate(domain)" 
                  class="action-btn primary"
                  :disabled="domain.isRevealing || !domain.revealIp"
                >
                  Reveal Domain
                </button>
              </div>
              
              <!-- Revealed: Show Update IP input, TTL input, and button -->
              <div v-if="domain.status === 'revealed'" class="domain-update">
                <input 
                  v-model="domain.newIp" 
                  type="text" 
                  placeholder="Enter new IP address"
                  class="ip-input"
                  :disabled="domain.isUpdating"
                />
                <input 
                  v-model.number="domain.updateTtl" 
                  type="number" 
                  min="0"
                  step="1"
                  placeholder="TTL (0=default)"
                  class="ttl-input"
                  :disabled="domain.isUpdating"
                  @input="domain.updateTtl = Math.max(0, Math.floor(Number(domain.updateTtl) || 0))"
                />
                <button 
                  @click="handleUpdate(domain)" 
                  class="action-btn primary"
                  :disabled="domain.isUpdating || !domain.newIp"
                >
                  Update IP
                </button>
              </div>
            </div>
          </div>
        </div>

        <div v-if="myDomains.length === 0" class="no-domains">
          You don't have any domains yet. Register one above!
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
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useWallet } from '../composables/useWallet.js';
import { useTransaction } from '../composables/useTransaction.js';
import { hashDomainWithSalt, generateTransactionSignature, hashTransaction } from '../services/crypto.service.js';
import { buildTransaction, computeTransactionID } from '../services/transaction.service.js';
import { sendTransaction, getMinerID, setMinerID, fetchRegisteredDomains, getSpendPlan } from '../services/api.service.js';
import { generateSalt } from '../utils/hash.js';
import { savePendingDomain, getPendingDomains, updateDomainStatus, removePendingDomain, clearAllPendingDomains } from '../utils/domainStorage.js';

const router = useRouter();
const isConnected = ref(false);
const connectionStatus = ref('Connecting...');

const { wallet, walletID, isWalletLoaded, loadWallet, createWallet } = useWallet();
const domainName = ref('');
const ttl = ref(0);
const isProcessing = ref(false);
const status = ref('');
const lastTxId = ref('');
const domains = ref([]);
const domainsLoaded = ref(false);
const myDomains = ref([]);
const backendMinerID = ref('');

const statusClass = computed(() => {
  if (status.value.includes('success') || status.value.includes('Success')) {
    return 'success';
  } else if (status.value.includes('error') || status.value.includes('Error')) {
    return 'error';
  }
  return '';
});

const statusMessage = computed(() => status.value);

async function syncMinerID() {
  const desiredID = wallet.value?.id;
  if (!desiredID) {
    return;
  }
  if (backendMinerID.value === desiredID) {
    return;
  }
  try {
    await setMinerID(desiredID);
    backendMinerID.value = desiredID;
    console.log('[Wallet] Synced miner ID with wallet ID:', desiredID);
  } catch (error) {
    console.error('[Wallet] Failed to sync miner ID:', error);
    status.value = `Failed to sync miner ID: ${error.message}`;
  }
}

async function applySpendPlan(tx) {
  const plan = await getSpendPlan(tx.from, tx.amount);
  tx.inputs = plan.inputs || [];
  tx.outputs = plan.outputs || [];
}

function loadMyDomains() {
  const ownerID = wallet.value?.id;
  if (!ownerID) return;
  
  // Load all domains from localStorage (both pending and revealed)
  const stored = getPendingDomains(ownerID);
  
  // Map to add input fields for IP addresses and TTL
  myDomains.value = stored.map(d => ({
    ...d,
    revealIp: d.ip || '', // IP for first_update (reveal)
    revealTtl: 0, // TTL for first_update
    newIp: d.ip || '', // IP for update
    updateTtl: 0, // TTL for update
    isRevealing: false,
    isUpdating: false
  }));
}

function clearPendingDomains() {
  if (!confirm('Are you sure you want to clear all domains? This cannot be undone.')) {
    return;
  }
  
  const ownerID = wallet.value?.id;
  if (ownerID) {
    clearAllPendingDomains(ownerID);
    myDomains.value = [];
    status.value = 'All domains cleared.';
  }
}

watch(
  () => wallet.value?.id,
  (newID) => {
    if (newID) {
      syncMinerID();
    }
  }
);

onMounted(async () => {
  // Auto-connect to backend and fetch miner ID
  try {
    const minerID = await getMinerID();
    isConnected.value = true;
    connectionStatus.value = 'Connected';
    console.log('[Wallet] Auto-connected to node with miner ID:', minerID);
    backendMinerID.value = minerID;
  } catch (error) {
    console.error('[Wallet] Failed to connect to backend:', error);
    isConnected.value = false;
    connectionStatus.value = 'Connection Failed';
    status.value = 'Failed to connect to backend. Please ensure the backend is running.';
  }
  
  // Load wallet after connection attempt - auto-create if doesn't exist
  const walletLoaded = await loadWallet();
  if (!walletLoaded) {
    console.log('[Wallet] No wallet found, creating new wallet...');
    try {
      await createWallet();
      console.log('[Wallet] New wallet created successfully');
    } catch (error) {
      console.error('[Wallet] Failed to create wallet:', error);
      status.value = 'Failed to create wallet. Please refresh the page.';
    }
  }

  await syncMinerID();
  
  // Load user's domains
  await fetchDomains();
  loadMyDomains();
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
  status.value = 'Creating commitment...';
  
  try {
    const salt = generateSalt();
    console.log('[DEBUG] Creating commitment - Domain:', domainName.value, 'Salt:', salt);
    const hashedDomain = await hashDomainWithSalt(domainName.value, salt);
    console.log('[DEBUG] Hashed commitment:', hashedDomain);
    
    const senderID = wallet.value?.id;
    if (!senderID) {
      status.value = 'Wallet ID not found. Please reload the page.';
      isProcessing.value = false;
      return;
    }
    
    const transaction = await buildTransaction({
      type: 'NameNew',
      walletID: senderID,
      fee: 1,
      payload: {
        commitment: hashedDomain,
        ttl: ttl.value || 0
      },
      pk: wallet.value.publicKey
    });

    await applySpendPlan(transaction);
    
    console.log('[DEBUG] Transaction payload:', transaction.payload);
    console.log('[DEBUG] Transaction payload type:', typeof transaction.payload);
    
    // Compute transaction ID before signing
    const completeTx = await computeTransactionID(transaction);
    
    const txHash = await hashTransaction(completeTx);
    const signature = await generateTransactionSignature(txHash, wallet.value.privateKey);
    
    status.value = 'Sending transaction to network...';
    const response = await sendTransaction(completeTx, signature);
    
    if (response && response.status === 'success') {
      lastTxId.value = completeTx.transactionID;
      
      // Save pending domain with salt and namenew txid for later reveal
      savePendingDomain(senderID, domainName.value, salt, hashedDomain, completeTx.transactionID);
      
      status.value = `Commitment created! Wait for it to be mined in a block before revealing. Check "My Domains" section below.`;
      domainName.value = '';
      
      // Refresh pending domains list
      loadMyDomains();
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

async function handleFirstUpdate(pending) {
  if (!isWalletLoaded.value) return;
  if (!pending.revealIp || !pending.revealIp.trim()) {
    status.value = 'Please enter an IP address';
    return;
  }
  
  pending.isRevealing = true;
  status.value = 'Revealing domain...';
  
  try {
    const senderID = wallet.value?.id;
    if (!senderID) {
      status.value = 'Wallet ID not found. Please reload the page.';
      pending.isRevealing = false;
      return;
    }
    
    console.log('[DEBUG] First update - Domain:', pending.domain, 'Salt:', pending.salt);
    
    const transaction = await buildTransaction({
      type: 'NameFirstUpdate',
      walletID: senderID,
      fee: 1,
      payload: {
        domain: pending.domain,
        salt: pending.salt,
        ip: pending.revealIp.trim(),
        ttl: pending.revealTtl || 0,
        txid: pending.txid
      },
      pk: wallet.value.publicKey
    });

    await applySpendPlan(transaction);
    
    console.log('[DEBUG] First update payload:', transaction.payload);
    
    const completeTx = await computeTransactionID(transaction);
    const txHash = await hashTransaction(completeTx);
    const signature = await generateTransactionSignature(txHash, wallet.value.privateKey);
    
    status.value = 'Sending reveal transaction...';
    const response = await sendTransaction(completeTx, signature);
    
    if (response && response.status === 'success') {
      lastTxId.value = completeTx.transactionID;
      status.value = `Domain "${pending.domain}" revealed successfully!`;
      
      // Update status and save IP to localStorage
      const key = `pending_domains_${senderID}`;
      const stored = JSON.parse(localStorage.getItem(key) || '[]');
      const updated = stored.map(d => 
        d.domain === pending.domain 
          ? { ...d, status: 'revealed', ip: pending.revealIp ? pending.revealIp.trim() : '' } 
          : d
      );
      localStorage.setItem(key, JSON.stringify(updated));
      
      loadMyDomains();
      fetchDomains();
    } else {
      status.value = `Reveal failed: ${response?.message || 'Unknown error'}`;
    }
  } catch (error) {
    console.error('[Wallet] First update error:', error);
    const msg = error.message || 'Unknown error occurred';
    const msgLower = msg.toLowerCase();
    status.value = `Error: ${msg}`;

    // Remove domain from localStorage if it has errors indicating it's invalid
    const shouldRemove = 
      msgLower.includes('domain already exists') ||
      msgLower.includes('already exists') ||
      msgLower.includes('already claimed') ||
      msgLower.includes('already registered') ||
      msgLower.includes('no matching') ||
      msgLower.includes('commitment mismatch') ||
      msgLower.includes('expired');

    if (shouldRemove) {
      const senderID = wallet.value?.id;
      if (senderID) {
        console.log(`[Wallet] Removing domain "${pending.domain}" from storage due to error`);
        removePendingDomain(senderID, pending.domain);
        loadMyDomains();
        fetchDomains();
      }
    }
  } finally {
    pending.isRevealing = false;
  }
}

async function handleUpdate(domain) {
  if (!isWalletLoaded.value) return;
  if (!domain.newIp || !domain.newIp.trim()) {
    status.value = 'Please enter an IP address';
    return;
  }
  
  domain.isUpdating = true;
  status.value = `Updating IP for ${domain.domain}...`;
  
  try {
    const senderID = wallet.value?.id;
    if (!senderID) {
      status.value = 'Wallet ID not found. Please reload the page.';
      domain.isUpdating = false;
      return;
    }
    
    const transaction = await buildTransaction({
      type: 'NameUpdate',
      walletID: senderID,
      fee: 1,
      payload: {
        domain: domain.domain,
        ip: domain.newIp.trim(),
        ttl: domain.updateTtl || 0
      },
      pk: wallet.value.publicKey
    });

    await applySpendPlan(transaction);
    
    const completeTx = await computeTransactionID(transaction);
    const txHash = await hashTransaction(completeTx);
    const signature = await generateTransactionSignature(txHash, wallet.value.privateKey);
    
    status.value = 'Sending update transaction...';
    const response = await sendTransaction(completeTx, signature);
    
    if (response && response.status === 'success') {
      lastTxId.value = completeTx.transactionID;
      status.value = `IP updated for "${domain.domain}"! Wait for it to be mined in a block.`;
      
      // Update the stored IP and clear input
      const key = `pending_domains_${senderID}`;
      const stored = JSON.parse(localStorage.getItem(key) || '[]');
      const updated = stored.map(d => 
        d.domain === domain.domain ? { ...d, ip: domain.newIp.trim() } : d
      );
      localStorage.setItem(key, JSON.stringify(updated));
      
      // Clear the input field
      domain.newIp = '';
      
      // Refresh domain list
      loadMyDomains();
    } else {
      status.value = `Update failed: ${response?.message || 'Unknown error'}`;
    }
  } catch (error) {
    console.error('[Wallet] Update error:', error);
    const msg = error.message || 'Unknown error occurred';
    const msgLower = msg.toLowerCase();
    status.value = `Error: ${msg}`;

    // Remove domain from localStorage if it's expired or non-existent
    const shouldRemove = 
      msgLower.includes('non-existent') ||
      msgLower.includes('expired') ||
      msgLower.includes('does not exist') ||
      msgLower.includes('not found') ||
      msgLower.includes('cannot update domain you do not own');

    if (shouldRemove) {
      const senderID = wallet.value?.id;
      if (senderID) {
        console.log(`[Wallet] Removing domain "${domain.domain}" from storage due to error`);
        removePendingDomain(senderID, domain.domain);
        loadMyDomains();
        fetchDomains();
      }
    }
  } finally {
    domain.isUpdating = false;
  }
}

async function fetchDomains() {
  try {
    const allDomains = await fetchRegisteredDomains();
    
    // Map the domain records to display format
    domains.value = allDomains.map(record => ({
      name: record.Domain || record.domain,
      owner: record.Owner || record.owner,
      ip: record.IP || record.ip
    }));
    
    domainsLoaded.value = true;
    console.log('[Wallet] Fetched domains:', domains.value);
  } catch (error) {
    console.error('[Wallet] Error fetching domains:', error);
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
.domain-block,
.domain-ip {
  display: flex;
  align-items: center;
}

.domain-update {
  display: flex;
  gap: 10px;
  margin-top: 12px;
  align-items: center;
}

.ip-input {
  flex: 1;
  padding: 10px 14px;
  border: 2px solid #e0e0e0;
  border-radius: 6px;
  font-size: 14px;
  transition: all 0.3s;
}

.ip-input:focus {
  outline: none;
  border-color: #667eea;
  box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
}

.ip-input:disabled {
  background: #f5f5f5;
  cursor: not-allowed;
}

.ttl-input {
  width: 140px;
  padding: 10px 14px;
  border: 2px solid #e0e0e0;
  border-radius: 6px;
  font-size: 14px;
  transition: all 0.3s;
}

.ttl-input:focus {
  outline: none;
  border-color: #667eea;
  box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
}

.ttl-input:disabled {
  background: #f5f5f5;
  cursor: not-allowed;
}

.form-hint {
  display: block;
  margin-top: 6px;
  color: #666;
  font-size: 13px;
  line-height: 1.4;
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

/* My Domains Section */
.my-domains-section {
  background: white;
  border-radius: 10px;
  padding: 30px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  margin-top: 30px;
}

.my-domains-section h2 {
  margin-top: 0;
  margin-bottom: 20px;
  color: #333;
  font-size: 22px;
}

.subsection {
  margin-bottom: 30px;
}

.subsection:last-child {
  margin-bottom: 0;
}

.subsection-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.subsection h3 {
  margin: 0;
  color: #667eea;
  font-size: 18px;
}

.clear-btn {
  padding: 6px 14px;
  background: #ff5252;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s;
}

.clear-btn:hover {
  background: #e04848;
  transform: translateY(-1px);
  box-shadow: 0 2px 6px rgba(255, 82, 82, 0.3);
}

.section-description {
  margin: 0 0 15px 0;
  color: #666;
  font-size: 14px;
  line-height: 1.5;
}

.domain-item.pending,
.domain-item.revealed {
  background: #f9f9f9;
  padding: 20px;
  border-radius: 8px;
  margin-bottom: 15px;
  border-left: 4px solid #667eea;
}

.domain-item.pending {
  border-left-color: #ff9800;
  background: #fff8f0;
}

.domain-item.revealed {
  border-left-color: #4caf50;
  background: #f1f8f4;
}

.domain-item.owned {
  border-left-color: #4caf50;
  background: #f1f8f4;
}

.domain-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 15px;
}

.domain-badge {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
}

.domain-badge.pending {
  background: #ffe0b2;
  color: #f57c00;
}

.domain-badge.revealed {
  background: #c8e6c9;
  color: #2e7d32;
}

.domain-actions {
  display: flex;
  gap: 10px;
  margin-top: 12px;
}

.action-btn {
  padding: 10px 20px;
  border: none;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s;
}

.action-btn.primary {
  background: #667eea;
  color: white;
}

.action-btn.primary:hover:not(:disabled) {
  background: #5568d3;
  transform: translateY(-1px);
  box-shadow: 0 4px 8px rgba(102, 126, 234, 0.3);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

</style>
