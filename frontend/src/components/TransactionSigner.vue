<template>
  <div class="transaction-signer">
    <h3>Transaction Preview</h3>
    
    <div class="tx-details">
      <div class="detail">
        <span class="label">Type:</span>
        <span class="value">{{ transaction.type }}</span>
      </div>
      <div class="detail">
        <span class="label">Source:</span>
        <span class="value mono">{{ transaction.source }}</span>
      </div>
      <div class="detail">
        <span class="label">Fee:</span>
        <span class="value">{{ transaction.fee }}</span>
      </div>
      <div class="detail">
        <span class="label">Nonce:</span>
        <span class="value">{{ transaction.nonce }}</span>
      </div>
      <div class="detail">
        <span class="label">Transaction ID:</span>
        <span class="value mono">{{ transaction.transactionID }}</span>
      </div>
    </div>
    
    <div v-if="status === 'idle'" class="actions">
      <button @click="handleSign" class="btn-primary" :disabled="isSigning">
        {{ isSigning ? 'Signing...' : 'Sign & Send' }}
      </button>
      <button @click="handleCancel" class="btn-cancel">Cancel</button>
    </div>
    
    <div v-if="status === 'signing'" class="status signing">
      🔐 Signing transaction...
    </div>
    
    <div v-if="status === 'sending'" class="status sending">
      📤 Sending to network...
    </div>
    
    <div v-if="status === 'success'" class="status success">
      ✅ Transaction sent successfully!
    </div>
    
    <div v-if="status === 'error'" class="status error">
      ❌ Error: {{ error }}
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useTransaction } from '../composables/useTransaction.js';

const props = defineProps({
  transaction: Object,
  privateKey: String
});

const emit = defineEmits(['transaction-sent', 'cancel']);
const { signAndSend, status, error } = useTransaction();

const isSigning = ref(false);

async function handleSign() {
  isSigning.value = true;
  try {
    const result = await signAndSend(props.privateKey);
    emit('transaction-sent', result);
  } catch (err) {
    console.error('Sign & send failed:', err);
  } finally {
    isSigning.value = false;
  }
}

function handleCancel() {
  emit('cancel');
}
</script>

<style scoped>
.transaction-signer {
  background: white;
  padding: 25px;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
  margin-top: 20px;
}

h3 {
  margin: 0 0 20px 0;
  color: #333;
}

.tx-details {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 20px;
  padding: 15px;
  background: #f9f9f9;
  border-radius: 4px;
}

.detail {
  display: flex;
  gap: 10px;
}

.label {
  font-weight: bold;
  min-width: 120px;
  color: #666;
}

.value {
  color: #333;
  word-break: break-all;
}

.value.mono {
  font-family: monospace;
  font-size: 12px;
}

.actions {
  display: flex;
  gap: 10px;
}

.btn-primary {
  padding: 12px 24px;
  background: #4CAF50;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 16px;
  cursor: pointer;
  flex: 1;
}

.btn-primary:hover:not(:disabled) {
  background: #45a049;
}

.btn-primary:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.btn-cancel {
  padding: 12px 24px;
  background: #f44336;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 16px;
  cursor: pointer;
}

.btn-cancel:hover {
  background: #d32f2f;
}

.status {
  padding: 15px;
  border-radius: 4px;
  text-align: center;
  font-weight: bold;
  margin-top: 20px;
}

.status.signing {
  background: #fff3e0;
  color: #e65100;
}

.status.sending {
  background: #e3f2fd;
  color: #0d47a1;
}

.status.success {
  background: #d4edda;
  color: #155724;
}

.status.error {
  background: #f8d7da;
  color: #721c24;
}
</style>
