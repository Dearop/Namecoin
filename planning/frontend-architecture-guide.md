# Frontend Architecture Guide - Crypto Wallet Implementation
**Project:** Peerster DNS Miners  
**Date:** November 24, 2025  
**Approach:** Vue.js Frontend (Option B)

---

## Table of Contents
1. [Project Structure](#1-project-structure)
2. [File Responsibilities](#2-file-responsibilities)
3. [Data Flow](#3-data-flow)
4. [Cryptographic Implementation](#4-cryptographic-implementation)
5. [Backend Integration](#5-backend-integration)
6. [State Management](#6-state-management)
7. [Implementation Order](#7-implementation-order)
8. [Development Tips](#8-development-tips)
9. [Security Checklist](#9-security-checklist)
10. [Example Components](#10-example-components)

---

## 1. Project Structure

### Complete Directory Layout

```
frontend/                      
├── package.json               (Dependencies: Vue 3, Vite, crypto libs)
├── vite.config.js             (Build configuration)
├── index.html                 (Entry HTML)
├── .gitignore                 (node_modules, dist, .env)
├── README.md                  (Frontend-specific docs)
├── public/                    (Static assets)
│   └── favicon.ico
├── src/
    ├── main.js                (Vue app entry point)
    ├── App.vue                (Root component)
    ├── components/            (UI Components)
    │   ├── WalletManager.vue  (Wallet creation/selection UI)
    │   ├── DomainCreator.vue  (Domain name input form)
    │   ├── TransactionSigner.vue (Sign & send UI)
    │   ├── TransactionList.vue   (History of transactions)
    │   └── StatusIndicator.vue   (Loading/success/error states)
    ├── services/              (Business Logic Layer)
    │   ├── wallet.service.js  (Key generation, storage)
    │   ├── crypto.service.js  (Hashing, signing logic)
    │   ├── transaction.service.js (Transaction builder)
    │   └── api.service.js     (Backend communication)
    ├── utils/                 (Pure Helper Functions)
    │   ├── hash.js            (SHA256, salt generation)
    │   ├── storage.js         (localStorage wrapper)
    │   └── validation.js      (Input validation)
    ├── composables/           (Vue 3 Composition API)
    │   ├── useWallet.js       (Wallet state management)
    │   └── useTransaction.js  (Transaction state management)
    ├── assets/                (Styles, images)
    │   └── main.css
    └── router/                (Optional: if multi-page)
        └── index.js
```

---

## 2. File Responsibilities

### Layer 1: Components (UI Layer)
**Purpose:** User interaction and display only

#### **WalletManager.vue**
```
Responsibilities:
- Button to generate new wallet
- Display wallet ID (public key)
- Show private key with copy button
- Load existing wallet from file/localStorage
- Export wallet to JSON file

Data it manages:
- walletExists: boolean
- walletID: string
- showPrivateKey: boolean (toggle visibility)

Methods it calls (from services):
- walletService.generateKeyPair()
- walletService.saveWallet()
- walletService.loadWallet()
```

#### **DomainCreator.vue**
```
Responsibilities:
- Input field for domain name
- Auto-generate and display salt
- Show computed hash(domain + salt)
- Fee input (default = 1, editable)
- "Create Domain" button

Data it manages:
- domainName: string
- salt: string (auto-generated)
- hashedValue: string (computed)
- fee: number

Methods it calls:
- cryptoService.generateSalt()
- cryptoService.hashDomainWithSalt()
- transactionService.buildTransaction()
```

#### **TransactionSigner.vue**
```
Responsibilities:
- Display transaction details preview (readonly)
- "Sign & Send" button
- Status display (pending, success, error)
- Transaction ID after submission

Data it manages:
- transaction: object (received as prop)
- isSigning: boolean
- status: string
- txID: string

Methods it calls:
- cryptoService.signTransaction()
- apiService.sendTransaction()
```

#### **TransactionList.vue** (Optional)
```
Responsibilities:
- Show list of all submitted transactions
- Display status of each
- Filter by status (pending/confirmed)

Data it manages:
- transactions: array (from localStorage)
```

#### **StatusIndicator.vue** (Reusable)
```
Responsibilities:
- Show loading spinner
- Success checkmark
- Error message display

Props:
- status: 'idle' | 'loading' | 'success' | 'error'
- message: string
```

---

### Layer 2: Services (Business Logic)
**Purpose:** Pure JavaScript logic, no Vue dependencies, fully testable

#### **services/wallet.service.js**
```javascript
Export functions:

- generateKeyPair() 
  → Returns: { publicKey: string, privateKey: string }
  → Uses: Web Crypto API or @noble/ed25519

- deriveWalletID(publicKey)
  → Returns: string (hex wallet identifier)
  → Uses: Hash of public key or direct conversion

- saveWallet(publicKey, privateKey)
  → Stores in localStorage (encrypted or plain)
  → Also triggers download of JSON file

- loadWallet()
  → Returns: { publicKey, privateKey } | null
  → Reads from localStorage

- exportWalletToFile(wallet)
  → Creates downloadable JSON file
  → Includes timestamp, version

- importWalletFromFile(file)
  → Parses uploaded JSON
  → Validates structure
  → Returns wallet object
```

#### **services/crypto.service.js**
```javascript
Export functions:

- generateSalt(length = 32)
  → Returns: hex string
  → Uses: crypto.getRandomValues()

- hashDomainWithSalt(domain, salt)
  → Returns: hex string
  → Uses: SHA256(domain + salt)

- hashTransaction(txObject)
  → Returns: transaction ID (hex string)
  → Hash order: type + source + fee + payload + nonce
  → Excludes signature field

- signTransaction(txHash, privateKey)
  → Returns: signature (hex string)
  → Uses: Ed25519 or ECDSA
  → Input: transaction hash, private key

- verifySignature(txHash, signature, publicKey)
  → Returns: boolean (for testing)
  → Verifies signature is valid
```

#### **services/transaction.service.js**
```javascript
Export functions:

- buildTransaction(params)
  → Input: { 
      type: string,
      walletID: string,
      fee: number,
      payload: object | string,
      nonce: number
    }
  → Returns: Transaction object (before signing)
  → Structure:
    {
      type: "name_new" | "name_firstupdate" | "name_update",
      source: walletID,
      fee: fee,
      payload: encodePayload(params.payload),
      nonce: nonce,
      transactionID: null  // computed later
    }

- computeTransactionID(tx)
  → Calls cryptoService.hashTransaction()
  → Sets tx.transactionID
  → Returns updated tx

- encodePayload(data)
  → Input: { domain, salt, hash } or other
  → Returns: encoded bytes/string for payload field
  → Format depends on transaction type

- incrementNonce()
  → Reads from localStorage
  → Increments counter
  → Saves back
  → Returns new nonce value

- validateTransaction(tx)
  → Returns: { valid: boolean, errors: string[] }
  → Checks: required fields, types, ranges
```

#### **services/api.service.js**
```javascript
Configuration:
const BASE_URL = 'http://localhost:8080' // or from env

Export functions:

- sendTransaction(tx, signature)
  → POST /namecoin/transaction
  → Body: { transaction: tx, signature: signature }
  → Returns: Promise<{ txID: string, status: string }>

- getTransactionStatus(txID)
  → GET /namecoin/transaction/:txID
  → Returns: Promise<{ status: string, confirmations: number }>

- getBlockchainState()
  → GET /blockchain
  → Returns: current blockchain info (for display)

Error handling:
- Wraps all fetch calls with try/catch
- Returns standardized error objects
- Includes retry logic for network failures
```

---

### Layer 3: Utils (Pure Helper Functions)
**Purpose:** Zero dependencies, pure functions, highly testable

#### **utils/hash.js**
```javascript
Export functions:

- sha256(input)
  → Input: string or Uint8Array
  → Returns: Promise<string> (hex)
  → Uses: crypto.subtle.digest()

- generateRandomSalt(length)
  → Returns: hex string
  → Uses: crypto.getRandomValues()
  → Crypto-secure randomness

- stringToBytes(str)
  → Converts string to Uint8Array
  → For crypto operations

- bytesToHex(bytes)
  → Converts Uint8Array to hex string
  → For display and storage

- hexToBytes(hex)
  → Converts hex string to Uint8Array
  → For crypto operations
```

#### **utils/storage.js**
```javascript
Export functions:

- setItem(key, value)
  → Wrapper around localStorage.setItem()
  → Handles JSON serialization
  → Catches quota exceeded errors

- getItem(key, defaultValue)
  → Wrapper around localStorage.getItem()
  → Handles JSON parsing
  → Returns defaultValue if not found

- removeItem(key)
  → Wrapper around localStorage.removeItem()

- clear()
  → Clears all app-specific keys
  → Preserves system keys

Specific helpers:
- saveWalletData(wallet)
- getWalletData()
- saveDomain(domainData)
- getDomains()
- saveNonce(nonce)
- getNonce()
```

#### **utils/validation.js**
```javascript
Export functions:

- isValidDomain(domain)
  → Returns: boolean
  → Regex: /^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$/

- isValidFee(fee)
  → Returns: boolean
  → Checks: number, positive, integer

- isValidHex(str)
  → Returns: boolean
  → Checks if valid hex string

- isValidWalletID(id)
  → Returns: boolean
  → Checks format and length
```

---

### Layer 4: Composables (Vue 3 State Management)
**Purpose:** Reactive state shared across components

#### **composables/useWallet.js**
```javascript
Export function: useWallet()

Returns reactive state and methods:
{
  // State
  wallet: ref({ publicKey: null, privateKey: null }),
  walletID: computed(() => derive from publicKey),
  isWalletLoaded: computed(() => wallet.value.publicKey !== null),
  
  // Methods
  async createWallet(),
  async loadWallet(),
  async exportWallet(),
  async importWallet(file),
  clearWallet()
}

Internally calls:
- walletService methods
- Updates reactive state
- Handles errors
```

#### **composables/useTransaction.js**
```javascript
Export function: useTransaction()

Returns reactive state and methods:
{
  // State
  currentTransaction: ref(null),
  isProcessing: ref(false),
  status: ref('idle'),
  error: ref(null),
  txHistory: ref([]),
  
  // Methods
  async createTransaction(params),
  async signAndSend(),
  async checkStatus(txID),
  clearTransaction()
}

Internally calls:
- transactionService methods
- cryptoService methods
- apiService methods
- Updates reactive state
```

---

## 3. Data Flow

### Complete Flow from User Action to Backend

```
[User Interface] → [Component] → [Composable] → [Service] → [Util] → [Backend]

═══════════════════════════════════════════════════════════════════════
STEP 1: Wallet Creation
═══════════════════════════════════════════════════════════════════════

User clicks "Create Wallet"
  ↓
WalletManager.vue → createWallet()
  ↓
useWallet.js → createWallet()
  ↓
walletService.generateKeyPair()
  ↓ (uses Web Crypto API)
Returns { publicKey, privateKey }
  ↓
walletService.deriveWalletID(publicKey)
  ↓
utils/hash.js → sha256(publicKey)
  ↓
walletService.saveWallet()
  ↓
utils/storage.js → setItem('wallet', data)
  ↓
useWallet state updates (reactive)
  ↓
WalletManager.vue displays wallet ID


═══════════════════════════════════════════════════════════════════════
STEP 2: Domain Name Creation (name_new)
═══════════════════════════════════════════════════════════════════════

User types domain "example.com"
  ↓
DomainCreator.vue → onInput()
  ↓ (auto-generates salt on mount)
cryptoService.generateSalt()
  ↓
utils/hash.js → generateRandomSalt(32)
  ↓ (user sees salt, must save it!)
User clicks "Create Domain"
  ↓
DomainCreator.vue → createDomain()
  ↓
cryptoService.hashDomainWithSalt(domain, salt)
  ↓
utils/hash.js → sha256(domain + salt)
  ↓ (now build transaction)
useTransaction.js → createTransaction({
  type: 'name_new',
  walletID: wallet.value.id,
  fee: 1,
  payload: { hash: hashedValue },
  nonce: transactionService.incrementNonce()
})
  ↓
transactionService.buildTransaction(params)
  ↓ (creates tx object)
transactionService.encodePayload(params.payload)
  ↓ (encodes hash for payload field)
transactionService.computeTransactionID(tx)
  ↓
cryptoService.hashTransaction(tx)
  ↓
utils/hash.js → sha256(type + source + fee + payload + nonce)
  ↓
Returns complete transaction object with ID
  ↓
useTransaction.currentTransaction updates
  ↓
DomainCreator emits event to parent
  ↓
Parent passes tx to TransactionSigner.vue


═══════════════════════════════════════════════════════════════════════
STEP 3: Transaction Signing & Sending
═══════════════════════════════════════════════════════════════════════

TransactionSigner.vue displays tx preview
  ↓
User clicks "Sign & Send"
  ↓
TransactionSigner.vue → signAndSend()
  ↓
useTransaction.js → signAndSend()
  ↓
cryptoService.signTransaction(
  currentTransaction.value.transactionID,
  wallet.value.privateKey
)
  ↓ (uses Web Crypto API for signing)
Returns signature (hex string)
  ↓
apiService.sendTransaction(tx, signature)
  ↓
POST http://localhost:8080/namecoin/transaction
Body: {
  transaction: {
    type: "name_new",
    source: "wallet_id_hex",
    fee: 1,
    payload: "hash_hex",
    nonce: 1,
    transactionID: "tx_id_hex"
  },
  signature: "signature_hex"
}
  ↓
[BACKEND RECEIVES]
  ↓
backend validates signature
  ↓
backend processes transaction
  ↓
backend returns { success: true, txID: "..." }
  ↓
apiService returns response
  ↓
useTransaction updates status to 'success'
  ↓
TransactionSigner.vue shows success message
  ↓
Transaction added to history (localStorage)
```

---

## 4. Cryptographic Implementation

### Recommended Library: @noble/ed25519

```bash
npm install @noble/ed25519
```

**Why Ed25519?**
- Fast, small signatures (64 bytes)
- Modern, secure (better than RSA)
- Pure JavaScript, no native dependencies
- Compatible with most backends

### Key Generation Example

```javascript
// In wallet.service.js
import * as ed from '@noble/ed25519';

async function generateKeyPair() {
  // Generate 32-byte private key
  const privateKey = ed.utils.randomPrivateKey();
  
  // Derive public key from private
  const publicKey = await ed.getPublicKey(privateKey);
  
  return {
    privateKey: bytesToHex(privateKey),
    publicKey: bytesToHex(publicKey)
  };
}
```

### Signing Example

```javascript
// In crypto.service.js
import * as ed from '@noble/ed25519';

async function signTransaction(txHash, privateKeyHex) {
  const privateKey = hexToBytes(privateKeyHex);
  const message = hexToBytes(txHash);
  
  // Sign the transaction hash
  const signature = await ed.sign(message, privateKey);
  
  return bytesToHex(signature);
}
```

### Hash Function Example

```javascript
// In utils/hash.js
async function sha256(input) {
  const encoder = new TextEncoder();
  const data = typeof input === 'string' 
    ? encoder.encode(input) 
    : input;
  
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);
  const hashArray = new Uint8Array(hashBuffer);
  
  return bytesToHex(hashArray);
}
```

### Helper Functions

```javascript
// In utils/hash.js
function bytesToHex(bytes) {
  return Array.from(bytes)
    .map(b => b.toString(16).padStart(2, '0'))
    .join('');
}

function hexToBytes(hex) {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return bytes;
}

function generateRandomSalt(length = 32) {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytesToHex(bytes);
}
```

---

## 5. Backend Integration

### New Files Needed

#### **File: `gui/httpnode/controller/namecoin.go`**

```go
package controller

import (
    "encoding/json"
    "net/http"
    "github.com/rs/zerolog"
    "go.dedis.ch/cs438/peer"
    "go.dedis.ch/cs438/types"
)

// NewNamecoinCtrl returns a new namecoin controller
func NewNamecoinCtrl(peer peer.Peer, log *zerolog.Logger) namecoinctrl {
    return namecoinctrl{
        peer: peer,
        log:  log,
    }
}

type namecoinctrl struct {
    peer peer.Peer
    log  *zerolog.Logger
}

type TransactionRequest struct {
    Transaction types.Tx `json:"transaction"`
    Signature   string   `json:"signature"`
}

type TransactionResponse struct {
    Success bool   `json:"success"`
    TxID    string `json:"txID,omitempty"`
    Status  string `json:"status,omitempty"`
    Error   string `json:"error,omitempty"`
}

func (n namecoinctrl) SubmitTransactionHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodPost:
            n.submitTransactionPost(w, r)
        case http.MethodOptions:
            w.Header().Set("Access-Control-Allow-Origin", "*")
            w.Header().Set("Access-Control-Allow-Headers", "*")
            return
        default:
            http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
            return
        }
    }
}

func (n namecoinctrl) submitTransactionPost(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Headers", "*")
    w.Header().Set("Content-Type", "application/json")

    var req TransactionRequest
    err := json.NewDecoder(r.Body).Decode(&req)
    if err != nil {
        n.log.Error().Err(err).Msg("failed to decode request")
        json.NewEncoder(w).Encode(TransactionResponse{
            Success: false,
            Error:   "Invalid request format",
        })
        return
    }

    // TODO: Verify signature here
    // verified := verifySignature(req.Transaction, req.Signature)
    // if !verified {
    //     json.NewEncoder(w).Encode(TransactionResponse{
    //         Success: false,
    //         Error:   "Invalid signature",
    //     })
    //     return
    // }

    // Submit transaction to peer
    // err = n.peer.SubmitTransaction(req.Transaction)
    // if err != nil {
    //     n.log.Error().Err(err).Msg("failed to submit transaction")
    //     json.NewEncoder(w).Encode(TransactionResponse{
    //         Success: false,
    //         Error:   err.Error(),
    //     })
    //     return
    // }

    response := TransactionResponse{
        Success: true,
        TxID:    string(req.Transaction.ID),
        Status:  "pending",
    }

    json.NewEncoder(w).Encode(response)
}
```

#### **Register in `gui/httpnode/httpnode.go`**

Add after the existing handlers (around line 96):

```go
namecoinctrl := controller.NewNamecoinCtrl(node, &log)
mux.Handle("/namecoin/transaction", 
    http.HandlerFunc(namecoinctrl.SubmitTransactionHandler()))
```

### API Request/Response Format

**POST /namecoin/transaction**

```json
Request:
{
  "transaction": {
    "type": "name_new",
    "source": "public_key_hex",
    "fee": 1,
    "payload": "base64_or_hex_encoded",
    "nonce": 1,
    "transactionID": "hash_of_tx_hex"
  },
  "signature": "ed25519_signature_hex"
}

Response (Success):
{
  "success": true,
  "txID": "transaction_id_hex",
  "status": "pending"
}

Response (Error):
{
  "success": false,
  "error": "Invalid signature"
}
```

---

## 6. State Management

### localStorage Structure

```javascript
// Key: "peerster_wallet"
{
  "publicKey": "hex_string",
  "privateKey": "hex_string_encrypted_or_plain",
  "walletID": "derived_id_hex",
  "createdAt": "2025-11-24T12:00:00Z"
}

// Key: "peerster_nonce"
0  // integer, increments

// Key: "peerster_domains"
[
  {
    "name": "example.com",
    "salt": "random_hex",
    "hash": "sha256_hex",
    "txID": "transaction_id_hex",
    "status": "pending" | "confirmed",
    "createdAt": "2025-11-24T12:00:00Z",
    "type": "name_new"
  }
]

// Key: "peerster_transactions"
[
  {
    "txID": "transaction_id_hex",
    "type": "name_new",
    "timestamp": "2025-11-24T12:00:00Z",
    "status": "pending" | "confirmed" | "failed",
    "details": { /* full tx object */ }
  }
]
```

### Storage Service Implementation

```javascript
// utils/storage.js
const STORAGE_PREFIX = 'peerster_';

export const StorageKeys = {
  WALLET: `${STORAGE_PREFIX}wallet`,
  NONCE: `${STORAGE_PREFIX}nonce`,
  DOMAINS: `${STORAGE_PREFIX}domains`,
  TRANSACTIONS: `${STORAGE_PREFIX}transactions`,
};

export function setItem(key, value) {
  try {
    const serialized = JSON.stringify(value);
    localStorage.setItem(key, serialized);
    return true;
  } catch (error) {
    console.error('Storage error:', error);
    return false;
  }
}

export function getItem(key, defaultValue = null) {
  try {
    const item = localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch (error) {
    console.error('Storage error:', error);
    return defaultValue;
  }
}

export function removeItem(key) {
  localStorage.removeItem(key);
}

export function clear() {
  Object.values(StorageKeys).forEach(key => {
    localStorage.removeItem(key);
  });
}

// Specific helpers
export function saveWalletData(wallet) {
  return setItem(StorageKeys.WALLET, wallet);
}

export function getWalletData() {
  return getItem(StorageKeys.WALLET);
}

export function saveNonce(nonce) {
  return setItem(StorageKeys.NONCE, nonce);
}

export function getNonce() {
  return getItem(StorageKeys.NONCE, 0);
}

export function incrementNonce() {
  const current = getNonce();
  const next = current + 1;
  saveNonce(next);
  return next;
}

export function saveDomain(domainData) {
  const domains = getItem(StorageKeys.DOMAINS, []);
  domains.push(domainData);
  return setItem(StorageKeys.DOMAINS, domains);
}

export function getDomains() {
  return getItem(StorageKeys.DOMAINS, []);
}
```

---

## 7. Implementation Order

### Phase 1: Project Setup (Day 1)

```bash
cd frontend/
npm init vue@latest .
# Select: Vue 3, No TypeScript, No Router (for now), No Pinia

npm install
npm install @noble/ed25519
npm run dev  # Test basic setup at http://localhost:5173
```

**Files to create:**
1. Update `vite.config.js` (add proxy to backend)
2. Clean up default `App.vue`
3. Create folder structure

```bash
mkdir -p src/{components,services,utils,composables}
```

### Phase 2: Core Utilities (Day 1-2)

**Build foundation first (bottom-up):**

#### 1. **`utils/hash.js`** ✓
- Implement `sha256()`
- Implement `bytesToHex()`, `hexToBytes()`
- Implement `generateRandomSalt()`
- **Test in console:** `sha256('test').then(console.log)`

#### 2. **`utils/storage.js`** ✓
- Implement `setItem()`, `getItem()`, `removeItem()`
- Implement specific helpers (wallet, nonce, domains)
- **Test:** Store and retrieve objects in console

#### 3. **`utils/validation.js`** ✓
- Implement `isValidDomain()`
- Implement `isValidFee()`, `isValidHex()`
- **Test:** Validate various inputs

### Phase 3: Services (Day 2-3)

**Build business logic (test without UI):**

#### 4. **`services/wallet.service.js`** ✓
- Implement `generateKeyPair()`
- Implement `deriveWalletID()`
- Implement `saveWallet()`, `loadWallet()`
- Implement `exportWalletToFile()`, `importWalletFromFile()`
- **Test:** Create wallet and log keys in console

#### 5. **`services/crypto.service.js`** ✓
- Implement `generateSalt()`
- Implement `hashDomainWithSalt()`
- Implement `hashTransaction()`
- Implement `signTransaction()`
- Implement `verifySignature()`
- **Test:** Hash "example.com" with salt, sign & verify

#### 6. **`services/transaction.service.js`** ✓
- Implement `buildTransaction()`
- Implement `computeTransactionID()`
- Implement `encodePayload()`
- Implement `incrementNonce()`
- Implement `validateTransaction()`
- **Test:** Create transaction object with mock data

#### 7. **`services/api.service.js`** ✓
- Implement `sendTransaction()`
- Implement `getTransactionStatus()`
- Implement error handling
- **Test:** Will fail until backend ready (use mock)

### Phase 4: Backend Endpoint (Day 3)

#### 8. **Create `gui/httpnode/controller/namecoin.go`** ✓
- Implement `SubmitTransactionHandler()`
- Add signature verification (TODO)
- Register route in `httpnode.go`
- **Test with curl:**
  ```bash
  curl -X POST http://localhost:8080/namecoin/transaction \
    -H "Content-Type: application/json" \
    -d '{"transaction": {...}, "signature": "..."}'
  ```

### Phase 5: Composables (Day 4)

#### 9. **`composables/useWallet.js`** ✓
- Wrap wallet service with reactive state
- Implement `createWallet()`, `loadWallet()`, etc.
- **Test:** Use in a minimal component

#### 10. **`composables/useTransaction.js`** ✓
- Wrap transaction services
- Implement full transaction flow
- **Test:** Full flow in console

### Phase 6: UI Components (Day 4-5)

#### 11. **`WalletManager.vue`** ✓
- Create/load wallet UI
- Display wallet info
- Export/import functionality
- **Test:** Wallet generation works

#### 12. **`DomainCreator.vue`** ✓
- Domain input form
- Auto-generate salt display
- Show computed hash
- Fee input
- **Test:** Form submission

#### 13. **`TransactionSigner.vue`** ✓
- Transaction preview
- Sign & send button
- Status display
- **Test:** End-to-end transaction

#### 14. **`App.vue`** ✓
- Wire all components together
- Handle flow between components
- Add navigation/routing
- **Test:** Complete user flow

### Phase 7: Polish (Day 5-6)

15. Add comprehensive error handling
16. Add loading states everywhere
17. Add success/error toast messages
18. Add transaction history view
19. Add CSS styling (responsive design)
20. Add input validation feedback
21. Test end-to-end flow multiple times
22. Add documentation/help text

---

## 8. Development Tips

### Vite Configuration

```javascript
// vite.config.js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/namecoin': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/blockchain': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
```

### Testing Strategy

```javascript
// Test services independently in browser console

// 1. Test hash utility
import { sha256 } from './utils/hash.js';
console.log(await sha256('test')); // Should output hash

// 2. Test salt generation
import { generateSalt } from './services/crypto.service.js';
console.log(await generateSalt()); // Should show random hex

// 3. Test key generation
import { generateKeyPair } from './services/wallet.service.js';
const keys = await generateKeyPair();
console.log('Public:', keys.publicKey);
console.log('Private:', keys.privateKey);

// 4. Test transaction building
import { buildTransaction } from './services/transaction.service.js';
const tx = buildTransaction({
  type: 'name_new',
  walletID: 'test_wallet_id',
  fee: 1,
  payload: { hash: 'test_hash' },
  nonce: 1
});
console.log('Transaction:', tx);
```

### Mock Backend During Development

```javascript
// In api.service.js
const MOCK_MODE = import.meta.env.DEV && false; // Toggle this

export async function sendTransaction(tx, signature) {
  if (MOCK_MODE) {
    // Simulate network delay
    await new Promise(resolve => setTimeout(resolve, 1000));
    return { 
      success: true, 
      txID: 'mock_tx_' + Date.now(),
      status: 'pending'
    };
  }
  
  // Real implementation
  const response = await fetch('/namecoin/transaction', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ transaction: tx, signature })
  });
  
  return response.json();
}
```

### Debugging Tips

```javascript
// Add to services for visibility
console.log('[WALLET] Generated keypair');
console.log('[TX] Built transaction:', tx);
console.log('[CRYPTO] Signature:', signature);
console.log('[API] Sending to backend:', payload);

// Use Vue DevTools browser extension
// Install: https://devtools.vuejs.org/
// Provides: Component inspector, state viewer, event tracker
```

### Environment Variables

```bash
# .env.development
VITE_API_BASE_URL=http://localhost:8080
VITE_ENABLE_MOCK=false

# .env.production
VITE_API_BASE_URL=https://api.production.com
VITE_ENABLE_MOCK=false
```

```javascript
// Access in code
const API_URL = import.meta.env.VITE_API_BASE_URL;
```

---

## 9. Security Checklist

### Critical Security Requirements

- [ ] **Private key NEVER sent to backend**
  - Only signatures are transmitted
  - All signing happens client-side
  
- [ ] **Salt generated with crypto.getRandomValues()**
  - Not `Math.random()` (predictable)
  - Crypto-secure randomness essential
  
- [ ] **Transaction hash excludes signature field**
  - Hash: type + source + fee + payload + nonce
  - Signature signs the hash
  
- [ ] **Backend signature verification**
  - Verify signature matches public key
  - Prevent transaction tampering
  
- [ ] **Input validation (frontend + backend)**
  - Domain format validation
  - Fee range checks
  - Hex string validation
  
- [ ] **HTTPS in production (not HTTP)**
  - Prevent man-in-the-middle attacks
  - Secure transmission
  
- [ ] **Private key storage warning**
  - Warn user about security
  - Recommend downloading backup
  - Option to encrypt in localStorage
  
- [ ] **Clear localStorage on logout**
  - Remove sensitive data
  - Prevent unauthorized access
  
- [ ] **Rate limiting on backend API**
  - Prevent spam/DoS
  - Implement per-IP limits
  
- [ ] **Transaction replay protection**
  - Nonce prevents resubmission
  - Increment after each transaction

### Additional Recommendations

- [ ] Consider encrypting private key in localStorage
- [ ] Add password protection for wallet
- [ ] Implement session timeouts
- [ ] Add two-factor authentication (advanced)
- [ ] Log all transactions for audit trail
- [ ] Implement transaction expiry timestamps
- [ ] Add CORS restrictions on backend
- [ ] Sanitize all user inputs
- [ ] Add CSP (Content Security Policy) headers
- [ ] Regular security audits

---

## 10. Example Components

### App.vue (Main Layout)

```vue
<template>
  <div id="app">
    <header>
      <h1>🔐 Peerster Crypto Wallet</h1>
      <p v-if="isWalletLoaded" class="wallet-id">
        Wallet: {{ walletID.substring(0, 16) }}...
      </p>
    </header>
    
    <main>
      <!-- Step 1: Create or load wallet -->
      <WalletManager 
        v-if="!isWalletLoaded" 
        @wallet-loaded="onWalletLoaded" 
      />
      
      <!-- Step 2: Domain creation interface -->
      <div v-else class="wallet-active">
        <div class="actions">
          <button @click="logout" class="logout-btn">Logout</button>
        </div>
        
        <DomainCreator 
          :wallet-id="walletID"
          @transaction-created="onTransactionCreated" 
        />
        
        <!-- Step 3: Transaction signing -->
        <TransactionSigner 
          v-if="currentTransaction" 
          :transaction="currentTransaction"
          :private-key="wallet.privateKey"
          @transaction-sent="onTransactionSent"
          @cancel="currentTransaction = null"
        />
        
        <!-- Transaction history -->
        <TransactionList :transactions="txHistory" />
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { useWallet } from './composables/useWallet';
import { useTransaction } from './composables/useTransaction';
import WalletManager from './components/WalletManager.vue';
import DomainCreator from './components/DomainCreator.vue';
import TransactionSigner from './components/TransactionSigner.vue';
import TransactionList from './components/TransactionList.vue';

const { wallet, walletID, isWalletLoaded, loadWallet, clearWallet } = useWallet();
const { currentTransaction, txHistory } = useTransaction();

onMounted(() => {
  // Try to load existing wallet
  loadWallet();
});

function onWalletLoaded() {
  console.log('[App] Wallet loaded successfully');
}

function onTransactionCreated(tx) {
  currentTransaction.value = tx;
  console.log('[App] Transaction created:', tx);
}

function onTransactionSent(result) {
  console.log('[App] Transaction sent:', result);
  currentTransaction.value = null;
  // Could show success toast here
}

function logout() {
  if (confirm('Are you sure you want to logout? Make sure you have backed up your private key.')) {
    clearWallet();
  }
}
</script>

<style scoped>
#app {
  max-width: 800px;
  margin: 0 auto;
  padding: 20px;
}

header {
  text-align: center;
  margin-bottom: 30px;
  padding-bottom: 20px;
  border-bottom: 2px solid #eee;
}

.wallet-id {
  font-family: monospace;
  color: #666;
  font-size: 14px;
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
  padding: 8px 16px;
  background: #f44336;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.logout-btn:hover {
  background: #d32f2f;
}
</style>
```

### WalletManager.vue

```vue
<template>
  <div class="wallet-manager">
    <h2>Get Started</h2>
    
    <div v-if="!showPrivateKey" class="options">
      <button @click="createNewWallet" :disabled="isLoading" class="btn-primary">
        Create New Wallet
      </button>
      
      <div class="separator">or</div>
      
      <div class="import-section">
        <label>Import Existing Wallet:</label>
        <input 
          type="file" 
          @change="importWallet" 
          accept=".json"
          :disabled="isLoading"
        />
      </div>
    </div>
    
    <!-- Show generated wallet -->
    <div v-else class="wallet-info">
      <div class="success-message">
        ✅ Wallet created successfully!
      </div>
      
      <div class="field">
        <label>Wallet ID (Public Key):</label>
        <div class="value-box">
          {{ walletID }}
          <button @click="copyToClipboard(walletID)" class="copy-btn">📋</button>
        </div>
      </div>
      
      <div class="field warning">
        <label>⚠️ Private Key (Save this securely!):</label>
        <div class="value-box">
          {{ wallet.privateKey }}
          <button @click="copyToClipboard(wallet.privateKey)" class="copy-btn">📋</button>
        </div>
        <p class="warning-text">
          This is your private key. Store it safely! Anyone with access to this 
          key can control your wallet.
        </p>
      </div>
      
      <div class="actions">
        <button @click="downloadWallet" class="btn-download">
          💾 Download Wallet File
        </button>
        <button @click="confirmAndContinue" class="btn-primary">
          Continue →
        </button>
      </div>
    </div>
    
    <StatusIndicator v-if="isLoading" status="loading" message="Creating wallet..." />
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useWallet } from '../composables/useWallet';
import StatusIndicator from './StatusIndicator.vue';

const emit = defineEmits(['wallet-loaded']);

const { wallet, walletID, createWallet, importWalletFromFile, exportWallet } = useWallet();
const isLoading = ref(false);
const showPrivateKey = ref(false);

async function createNewWallet() {
  isLoading.value = true;
  try {
    await createWallet();
    showPrivateKey.value = true;
  } catch (error) {
    console.error('[WalletManager] Error creating wallet:', error);
    alert('Failed to create wallet: ' + error.message);
  } finally {
    isLoading.value = false;
  }
}

async function importWallet(event) {
  const file = event.target.files[0];
  if (!file) return;
  
  isLoading.value = true;
  try {
    await importWalletFromFile(file);
    emit('wallet-loaded');
  } catch (error) {
    console.error('[WalletManager] Error importing wallet:', error);
    alert('Failed to import wallet: ' + error.message);
  } finally {
    isLoading.value = false;
  }
}

function downloadWallet() {
  exportWallet();
}

function copyToClipboard(text) {
  navigator.clipboard.writeText(text);
  alert('Copied to clipboard!');
}

function confirmAndContinue() {
  const confirmed = confirm(
    'Have you saved your private key? You will not be able to recover it later.'
  );
  if (confirmed) {
    emit('wallet-loaded');
  }
}
</script>

<style scoped>
.wallet-manager {
  background: white;
  padding: 30px;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.1);
}

.options {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
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

.btn-primary:hover {
  background: #45a049;
}

.btn-primary:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.separator {
  color: #999;
  font-style: italic;
}

.import-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.wallet-info {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.success-message {
  padding: 15px;
  background: #d4edda;
  color: #155724;
  border-radius: 4px;
  text-align: center;
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

.copy-btn {
  padding: 5px 10px;
  background: white;
  border: 1px solid #ddd;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
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

.btn-download {
  padding: 10px 20px;
  background: #2196F3;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.btn-download:hover {
  background: #1976D2;
}
</style>
```

### DomainCreator.vue

```vue
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
          :disabled="isProcessing"
          required
        />
        <span v-if="domainError" class="error">{{ domainError }}</span>
      </div>
      
      <div class="field">
        <label>Salt (Auto-generated):</label>
        <div class="value-box">
          {{ salt }}
          <button type="button" @click="regenerateSalt" class="regenerate-btn">
            🔄
          </button>
        </div>
        <p class="info-text">
          ⚠️ Save this salt! You'll need it for future operations.
        </p>
      </div>
      
      <div class="field">
        <label>Hash (domain + salt):</label>
        <div class="value-box computed">
          {{ hashedValue || 'Enter domain name...' }}
        </div>
      </div>
      
      <div class="field">
        <label for="fee">Fee:</label>
        <input 
          id="fee"
          v-model.number="fee" 
          type="number"
          min="1"
          :disabled="isProcessing"
          required
        />
      </div>
      
      <button 
        type="submit" 
        class="btn-primary"
        :disabled="!canSubmit"
      >
        {{ isProcessing ? 'Creating...' : 'Create Domain Transaction' }}
      </button>
    </form>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue';
import { generateSalt, hashDomainWithSalt } from '../services/crypto.service';
import { buildTransaction, computeTransactionID } from '../services/transaction.service';
import { isValidDomain, isValidFee } from '../utils/validation';

const props = defineProps({
  walletId: {
    type: String,
    required: true
  }
});

const emit = defineEmits(['transaction-created']);

const domainName = ref('');
const salt = ref('');
const fee = ref(1);
const hashedValue = ref('');
const isProcessing = ref(false);
const domainError = ref('');

const canSubmit = computed(() => {
  return !isProcessing.value && 
         domainName.value && 
         salt.value && 
         hashedValue.value &&
         !domainError.value;
});

// Auto-generate salt on mount
onMounted(() => {
  regenerateSalt();
});

// Watch domain name and recompute hash
watch([domainName, salt], async ([newDomain, newSalt]) => {
  if (newDomain && newSalt) {
    // Validate domain
    if (!isValidDomain(newDomain)) {
      domainError.value = 'Invalid domain format';
      hashedValue.value = '';
      return;
    }
    domainError.value = '';
    
    // Compute hash
    try {
      hashedValue.value = await hashDomainWithSalt(newDomain, newSalt);
    } catch (error) {
      console.error('[DomainCreator] Error hashing:', error);
      hashedValue.value = '';
    }
  } else {
    hashedValue.value = '';
  }
});

function regenerateSalt() {
  salt.value = generateSalt(32);
}

async function handleSubmit() {
  if (!canSubmit.value) return;
  
  // Validate fee
  if (!isValidFee(fee.value)) {
    alert('Fee must be a positive integer');
    return;
  }
  
  isProcessing.value = true;
  
  try {
    // Build transaction
    const tx = buildTransaction({
      type: 'name_new',
      walletID: props.walletId,
      fee: fee.value,
      payload: { 
        hash: hashedValue.value,
        domain: domainName.value,  // For reference
        salt: salt.value           // For reference
      },
      nonce: 0  // Will be set by service
    });
    
    // Compute transaction ID
    const completeTx = await computeTransactionID(tx);
    
    // Emit to parent
    emit('transaction-created', {
      transaction: completeTx,
      metadata: {
        domain: domainName.value,
        salt: salt.value,
        hash: hashedValue.value
      }
    });
    
    console.log('[DomainCreator] Transaction created:', completeTx);
    
  } catch (error) {
    console.error('[DomainCreator] Error creating transaction:', error);
    alert('Failed to create transaction: ' + error.message);
  } finally {
    isProcessing.value = false;
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

input:focus {
  outline: none;
  border-color: #4CAF50;
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

.value-box.computed {
  background: #e3f2fd;
  color: #1976d2;
}

.regenerate-btn {
  padding: 5px 10px;
  background: white;
  border: 1px solid #ddd;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
}

.info-text {
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
  transition: background 0.3s;
}

.btn-primary:hover:not(:disabled) {
  background: #45a049;
}

.btn-primary:disabled {
  background: #ccc;
  cursor: not-allowed;
}
</style>
```

---

## Summary

### Key Technologies
- **Framework:** Vue 3 with Composition API
- **Build Tool:** Vite
- **Crypto:** @noble/ed25519 + Web Crypto API
- **Storage:** localStorage
- **Backend:** Go with new Namecoin controller

### Architecture Principles
1. **Separation of Concerns** - UI ≠ Logic ≠ Data
2. **Layered Design** - Components → Composables → Services → Utils
3. **Testability** - Pure functions, minimal dependencies
4. **Security First** - Client-side signing, never expose private keys
5. **Progressive Enhancement** - Build bottom-up, test each layer

### Implementation Strategy
1. Start with utilities (hash, storage)
2. Build services (wallet, crypto, transaction)
3. Create backend endpoint
4. Add state management (composables)
5. Build UI components
6. Polish and test end-to-end

### Next Steps
1. Run `npm init vue@latest` in `frontend/`
2. Install `@noble/ed25519`
3. Create folder structure
4. Implement Phase 1: Utils
5. Continue phase by phase
6. Test thoroughly at each step

---

**Document Version:** 1.0  
**Last Updated:** November 24, 2025  
**Author:** Frontend Architecture Planning
