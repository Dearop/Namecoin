# Peerster Crypto Wallet Frontend

A simple Vue 3 frontend for the Peerster Crypto Wallet that handles domain name transactions.

## Setup

```bash
npm install
npm run dev
```

## Features

- Create/Import wallet (Ed25519 keypairs)
- Generate salted domain hashes
- Create and sign transactions
- Send transactions to backend

## Structure

- `src/components/` - Vue components
- `src/composables/` - Vue 3 Composition API state management
- `src/services/` - Business logic (wallet, crypto, transactions, API)
- `src/utils/` - Pure utility functions (hash, storage, validation)

## Development

The app runs on `http://localhost:5173` and proxies API requests to `http://localhost:8080`.
