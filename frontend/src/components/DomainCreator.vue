<template>
  <div class="domain-creator">
    <h2>Create Domain</h2>
    
    <form @submit.prevent="handleSubmit">
      <div class="field">
        <label for="domain">Domain Name:</label>
        <input 
          id="domain"
          v-model="domainName" 
          type="text"
          placeholder="example.com"
          required
        />
        <span v-if="error" class="error">{{ error }}</span>
      </div>
      
      <div class="field">
        <label>Salt (auto-generated):</label>
        <div class="value-box">
          {{ salt }}
          <button type="button" @click="regenerateSalt" class="small-btn">🔄</button>
        </div>
        <p class="info">⚠️ Save this salt! You'll need it later.</p>
      </div>
      
      <div class="field">
        <label>Hash (domain + salt):</label>
        <div class="value-box hash">
          {{ hashedValue || 'Calculating...' }}
        </div>
      </div>
      
      <div class="field">
        <label for="fee">Fee:</label>
        <input 
          id="fee"
          v-model.number="fee" 
          type="number"
          min="1"
          required
        />
      </div>
      
      <button type="submit" class="btn-primary" :disabled="!canSubmit">
        Create Transaction
      </button>
    </form>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue';
import { generateSalt, hashDomainWithSalt } from '../services/crypto.service.js';
import { isValidDomain } from '../utils/validation.js';
import { useTransaction } from '../composables/useTransaction.js';

const props = defineProps({
  walletId: String
});

const emit = defineEmits(['transaction-created']);
const { createTransaction } = useTransaction();

const domainName = ref('');
const salt = ref('');
const fee = ref(1);
const hashedValue = ref('');
const error = ref('');

const canSubmit = computed(() => {
  return domainName.value && salt.value && hashedValue.value && !error.value;
});

onMounted(() => {
  regenerateSalt();
});

watch([domainName, salt], async ([newDomain, newSalt]) => {
  if (newDomain && newSalt) {
    if (!isValidDomain(newDomain)) {
      error.value = 'Invalid domain format';
      hashedValue.value = '';
      return;
    }
    error.value = '';
    
    try {
      hashedValue.value = await hashDomainWithSalt(newDomain, newSalt);
    } catch (err) {
      console.error('Hash error:', err);
      hashedValue.value = '';
    }
  } else {
    hashedValue.value = '';
  }
});

function regenerateSalt() {
  salt.value = generateSalt();
}

async function handleSubmit() {
  if (!canSubmit.value) return;
  
  try {
    const tx = await createTransaction({
      type: 'first_update',
      walletID: props.walletId,
      fee: fee.value,
      payload: {
        hash: hashedValue.value,
        domain: domainName.value,
        salt: salt.value
      }
    });
    
    emit('transaction-created', {
      transaction: tx,
      metadata: {
        domain: domainName.value,
        salt: salt.value,
        hash: hashedValue.value
      }
    });
  } catch (err) {
    alert('Failed to create transaction: ' + err.message);
  }
}
</script>

<style scoped>
.domain-creator {
  background: white;
  padding: 25px;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
}

form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

label {
  font-weight: bold;
  color: #333;
}

input[type="text"],
input[type="number"] {
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
}

.value-box {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  background: #f5f5f5;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}

.value-box.hash {
  background: #e3f2fd;
  color: #1976d2;
}

.small-btn {
  padding: 5px 10px;
  background: white;
  border: 1px solid #ddd;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
}

.info {
  margin: 0;
  font-size: 14px;
  color: #ff9800;
}

.error {
  color: #d32f2f;
  font-size: 14px;
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
</style>
