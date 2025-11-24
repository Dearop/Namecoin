# 🧪 Frontend Testing Guide

## Quick Start

### 1. Start the Frontend

```bash
cd frontend
./start.sh
# Or manually: npm run dev
```

The app will run on `http://localhost:5173`

### 2. Start the Backend (in another terminal)

Make sure your Go backend is running on port 8080:

```bash
# From project root
make run
# Or however you normally start your Go backend
```

## Testing the Flow

### Step 1: Create Wallet

1. Open `http://localhost:5173`
2. Click "Create New Wallet"
3. You'll see:
   - Wallet ID (public key)
   - Private Key (⚠️ SAVE THIS!)
4. Click "💾 Download" to save wallet file
5. Click "Continue →"

### Step 2: Create Domain Transaction

1. Enter a domain name (e.g., `example.com`)
2. A random salt is auto-generated
3. The hash (SHA256 of domain + salt) is computed automatically
4. Set fee (default: 1)
5. Click "Create Transaction"

### Step 3: Sign & Send

1. Review transaction details:
   - Type: name_new
   - Source: your wallet ID
   - Fee: 1
   - Nonce: auto-incremented
   - Transaction ID: computed hash
2. Click "Sign & Send"
3. Transaction is:
   - Signed with your private key (Ed25519)
   - Sent to backend at `/namecoin/transaction`
4. You'll see success message with TX ID

## Testing Without Backend

If your backend isn't ready, you can test the crypto:

```javascript
// Open browser console at http://localhost:5173

// Test hash generation
import { sha256 } from './src/utils/hash.js';
await sha256('test'); // Should return hash

// Test key generation (already works in UI)
// Just create a wallet and check console logs
```

## Troubleshooting

### "Cannot connect to backend"
- Make sure Go backend is running on port 8080
- Check CORS is enabled (already done in namecoin.go)

### "Invalid signature"
- This means backend signature verification is active
- Check that Ed25519 algorithm matches between frontend/backend

### Salt not generating
- Check browser console for errors
- Make sure crypto.getRandomValues is available (HTTPS or localhost)

### Transaction not sending
- Open Network tab in DevTools
- Check if POST to `/namecoin/transaction` is successful
- Backend should respond with `{success: true, txID: "...", status: "pending"}`

## Features Implemented

✅ **Wallet Management**
- Ed25519 key generation
- Export/import wallet JSON
- LocalStorage persistence

✅ **Crypto Operations**
- SHA256 hashing (domain + salt)
- Ed25519 digital signatures
- Transaction ID computation

✅ **Transaction Flow**
- Build transaction object
- Compute transaction ID
- Sign transaction
- Send to backend

✅ **UI Components**
- WalletManager (create/import)
- DomainCreator (form with auto-hash)
- TransactionSigner (preview + sign)

## File Structure

```
frontend/
├── src/
│   ├── components/          # Vue UI components
│   │   ├── WalletManager.vue
│   │   ├── DomainCreator.vue
│   │   └── TransactionSigner.vue
│   ├── composables/         # Vue state management
│   │   ├── useWallet.js
│   │   └── useTransaction.js
│   ├── services/            # Business logic
│   │   ├── wallet.service.js
│   │   ├── crypto.service.js
│   │   ├── transaction.service.js
│   │   └── api.service.js
│   ├── utils/               # Pure functions
│   │   ├── hash.js
│   │   ├── storage.js
│   │   └── validation.js
│   ├── App.vue              # Main app
│   └── main.js              # Entry point
├── package.json
├── vite.config.js
└── index.html
```

## Next Steps

### Frontend Enhancements
- [ ] Add transaction history view
- [ ] Add loading spinners
- [ ] Add better error messages
- [ ] Add form validation feedback
- [ ] Add transaction status polling
- [ ] Support name_firstupdate and name_update

### Backend Integration
- [ ] Implement signature verification in Go
- [ ] Process transactions in Paxos consensus
- [ ] Return transaction status endpoint
- [ ] Add transaction to blockchain

### Security
- [ ] Add private key encryption option
- [ ] Add session timeout
- [ ] Add transaction confirmation dialog
- [ ] Add rate limiting

## Browser Compatibility

✅ Chrome/Edge (latest)
✅ Firefox (latest)
✅ Safari (latest)

Requires:
- Web Crypto API (for SHA256 and random generation)
- ES6 modules support
- LocalStorage

## Development Notes

### Hot Module Replacement (HMR)
Vite provides HMR - just save files and see changes instantly.

### Console Logs
Look for prefixed logs:
- `[Wallet]` - wallet operations
- `[Crypto]` - cryptographic operations
- `[TX]` - transaction operations
- `[API]` - backend communication
- `[App]` - main app events

### Storage Keys
LocalStorage keys used:
- `peerster_wallet` - wallet data
- `peerster_nonce` - transaction counter
- `peerster_domains` - domain list
- `peerster_transactions` - tx history

Clear with: `localStorage.clear()`

## API Contract

### POST /namecoin/transaction

**Request:**
```json
{
  "transaction": {
    "type": "name_new",
    "source": "wallet_public_key_hex",
    "fee": 1,
    "payload": "{\"hash\":\"...\",\"domain\":\"example.com\",\"salt\":\"...\"}",
    "nonce": 1,
    "transactionID": "tx_hash_hex"
  },
  "signature": "ed25519_signature_hex"
}
```

**Response (Success):**
```json
{
  "success": true,
  "txID": "transaction_id",
  "status": "pending"
}
```

**Response (Error):**
```json
{
  "success": false,
  "error": "error message"
}
```
