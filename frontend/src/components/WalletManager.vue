<template>
  <div class="wallet-manager">
    <h2>Wallet Setup</h2>
    
    <div v-if="!showKeys" class="options">
      <button @click="createNewWallet" :disabled="loading" class="btn-primary">
        {{ loading ? 'Creating...' : 'Create New Wallet' }}
      </button>
      
      <p class="separator">or</p>
      
      <div class="import-section">
        <label for="wallet-file">Import Wallet:</label>
        <input 
          id="wallet-file"
          type="file" 
          @change="handleImport" 
          accept=".json"
          :disabled="loading"
        />
      </div>
    </div>
    
    <div v-else class="wallet-info">
      <div class="success">✅ Wallet Created!</div>
      
      <div class="field">
        <label>Wallet ID:</label>
        <div class="value">{{ walletID }}</div>
      </div>
      
      <div class="field warning">
        <label>⚠️ Private Key (SAVE THIS!):</label>
        <div class="value">{{ wallet.privateKey }}</div>
        <p class="warning-text">Store this securely. You cannot recover it!</p>
      </div>
      
      <div class="actions">
        <button @click="download" class="btn-secondary">💾 Download</button>
        <button @click="continueToApp" class="btn-primary">Continue →</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useWallet } from '../composables/useWallet.js';

const emit = defineEmits(['wallet-loaded']);
const { wallet, walletID, createWallet, importWallet, exportWallet } = useWallet();

const loading = ref(false);
const showKeys = ref(false);

async function createNewWallet() {
  loading.value = true;
  try {
    await createWallet();
    showKeys.value = true;
  } catch (error) {
    alert('Failed to create wallet: ' + error.message);
  } finally {
    loading.value = false;
  }
}

async function handleImport(event) {
  const file = event.target.files[0];
  if (!file) return;
  
  loading.value = true;
  try {
    await importWallet(file);
    emit('wallet-loaded');
  } catch (error) {
    alert('Failed to import wallet: ' + error.message);
  } finally {
    loading.value = false;
  }
}

function download() {
  exportWallet();
}

function continueToApp() {
  emit('wallet-loaded');
}
</script>

<style scoped>
.wallet-manager {
  background: white;
  padding: 30px;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
  max-width: 600px;
  margin: 0 auto;
}

h2 {
  margin: 0 0 20px 0;
  color: #333;
}

.options {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
}

.btn-primary {
  padding: 12px 24px;
  background: #4CAF50;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 16px;
  cursor: pointer;
}

.btn-primary:hover:not(:disabled) {
  background: #45a049;
}

.btn-primary:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.btn-secondary {
  padding: 10px 20px;
  background: #2196F3;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.separator {
  color: #999;
  margin: 10px 0;
}

.import-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.wallet-info {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.success {
  padding: 15px;
  background: #d4edda;
  color: #155724;
  border-radius: 4px;
  text-align: center;
  font-weight: bold;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field.warning {
  border: 2px solid #ff9800;
  padding: 15px;
  border-radius: 4px;
  background: #fff3e0;
}

label {
  font-weight: bold;
  color: #333;
}

.value {
  padding: 10px;
  background: #f5f5f5;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}

.warning-text {
  margin: 10px 0 0;
  color: #d32f2f;
  font-size: 14px;
}

.actions {
  display: flex;
  gap: 10px;
  justify-content: space-between;
}
</style>
