<template>
  <div id="app">
    <header>
      <h1>🔐 Peerster Crypto Wallet</h1>
      <p v-if="isWalletLoaded" class="wallet-id">
        Wallet: {{ walletID?.substring(0, 16) }}...
      </p>
    </header>
    
    <main>
      <WalletManager 
        v-if="!isWalletLoaded" 
        @wallet-loaded="onWalletLoaded" 
      />
      
      <div v-else class="wallet-active">
        <div class="actions">
          <button @click="logout" class="logout-btn">Logout</button>
        </div>
        
        <DomainCreator 
          :wallet-id="walletID"
          @transaction-created="onTransactionCreated" 
        />
        
        <TransactionSigner 
          v-if="currentTransaction" 
          :transaction="currentTransaction"
          :private-key="wallet.privateKey"
          @transaction-sent="onTransactionSent"
          @cancel="currentTransaction = null"
        />
      </div>
    </main>
  </div>
</template>

<script setup>
import { onMounted } from 'vue';
import { useWallet } from './composables/useWallet.js';
import { useTransaction } from './composables/useTransaction.js';
import WalletManager from './components/WalletManager.vue';
import DomainCreator from './components/DomainCreator.vue';
import TransactionSigner from './components/TransactionSigner.vue';

const { wallet, walletID, isWalletLoaded, loadWallet, clearWallet } = useWallet();
const { currentTransaction } = useTransaction();

onMounted(() => {
  loadWallet();
});

function onWalletLoaded() {
  console.log('[App] Wallet loaded');
}

function onTransactionCreated(data) {
  currentTransaction.value = data.transaction;
  console.log('[App] Transaction created:', data);
}

function onTransactionSent(result) {
  console.log('[App] Transaction sent:', result);
  alert('Transaction sent successfully! TX ID: ' + result.txID);
  currentTransaction.value = null;
}

function logout() {
  if (confirm('Logout? Make sure you saved your private key!')) {
    clearWallet();
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
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  min-height: 100vh;
  padding: 20px;
}

#app {
  max-width: 800px;
  margin: 0 auto;
}

header {
  text-align: center;
  margin-bottom: 30px;
  padding: 20px;
  background: white;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
}

h1 {
  color: #333;
  margin-bottom: 10px;
}

.wallet-id {
  font-family: monospace;
  color: #666;
  font-size: 14px;
}

main {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.wallet-active {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.actions {
  display: flex;
  justify-content: flex-end;
}

.logout-btn {
  padding: 10px 20px;
  background: #f44336;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
}

.logout-btn:hover {
  background: #d32f2f;
}
</style>
