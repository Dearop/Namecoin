<template>
  <div class="home-container">
    <div class="connect-box">
      <h1>CS438 - Peerster Wallet</h1>
      <p class="subtitle">Enter the peer's proxy address</p>
      
      <form @submit.prevent="connect" class="connect-form">
        <input 
          v-model="proxyAddr" 
          type="text" 
          id="addr" 
          placeholder="127.0.0.1:8080"
          class="proxy-input"
          required
        />
        <button type="submit" class="connect-btn">Connect</button>
      </form>

      <div v-if="error" class="error-message">
        {{ error }}
      </div>

      <div class="instructions">
        <h3>Quick Start:</h3>
        <ol>
          <li>Start a peer: <code>cd gui && go run gui.go start</code></li>
          <li>Copy the proxy address from the log output</li>
          <li>Enter it above and click "Connect"</li>
        </ol>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { getMinerID } from '../services/api.service.js';

const router = useRouter();
const proxyAddr = ref('127.0.0.1:8080');
const error = ref('');

async function connect() {
  error.value = '';
  
  if (!proxyAddr.value) {
    error.value = 'Please enter a proxy address';
    return;
  }

  // Validate format (basic check)
  const addrPattern = /^[\w\.-]+:\d+$/;
  if (!addrPattern.test(proxyAddr.value)) {
    error.value = 'Invalid address format. Expected format: host:port (e.g., 127.0.0.1:8080)';
    return;
  }

  // Store proxy address
  localStorage.setItem('proxyAddr', proxyAddr.value);
  
  // Fetch and store miner ID from the backend
  try {
    const minerID = await getMinerID();
    localStorage.setItem('minerID', minerID);
    console.log('[Home] Connected to node with miner ID:', minerID);
    
    // Navigate to wallet
    router.push('/wallet');
  } catch (err) {
    error.value = `Failed to connect: ${err.message}`;
    localStorage.removeItem('proxyAddr');
  }
}
</script>

<style scoped>
.home-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 20px;
}

.connect-box {
  background: white;
  padding: 40px;
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  max-width: 500px;
  width: 100%;
}

h1 {
  color: #333;
  margin: 0 0 10px 0;
  font-size: 28px;
  text-align: center;
}

.subtitle {
  color: #666;
  text-align: center;
  margin-bottom: 30px;
  font-size: 16px;
}

.connect-form {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
}

.proxy-input {
  flex: 1;
  padding: 12px 16px;
  border: 2px solid #e0e0e0;
  border-radius: 6px;
  font-size: 16px;
  transition: border-color 0.3s;
}

.proxy-input:focus {
  outline: none;
  border-color: #667eea;
}

.connect-btn {
  padding: 12px 30px;
  background: #667eea;
  color: white;
  border: none;
  border-radius: 6px;
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.3s;
}

.connect-btn:hover {
  background: #5568d3;
}

.error-message {
  padding: 12px;
  background: #fee;
  color: #c33;
  border-radius: 6px;
  margin-bottom: 20px;
  font-size: 14px;
}

.instructions {
  margin-top: 30px;
  padding-top: 30px;
  border-top: 1px solid #e0e0e0;
}

.instructions h3 {
  color: #333;
  font-size: 18px;
  margin-bottom: 15px;
}

.instructions ol {
  margin: 0;
  padding-left: 20px;
  color: #666;
  line-height: 1.8;
}

.instructions code {
  background: #f5f5f5;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
  font-size: 14px;
  color: #667eea;
}
</style>
