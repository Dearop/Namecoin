# Namecoin Frontend

A Vue 3 frontend for the Peerster Crypto Wallet that handles domain name transactions with a connection-based architecture similar to the backend's CLI/Web GUI system.

## Quick Start

1. **Start a backend peer:**
```bash
cd gui
go run gui.go start --proxyaddr 127.0.0.1:8080
```

2. **Start the frontend:**
```bash
cd frontend
npm install
npm run dev
```

3. **Connect to peer:**
   - Open `http://localhost:5173`
   - Enter the proxy address from the backend logs (e.g., `127.0.0.1:8080`)
   - Click "Connect"

## Architecture

The frontend follows a similar pattern to the backend's HTTP proxy system:

```
Views (like node.html)
  ↓
Composables (like controllers)
  ↓
Services (like httpnode handlers)
  ↓
HTTP API calls to backend
  ↓
Backend HTTP Proxy
  ↓
Peer network
```

## Structure

```
src/
├── router/              # Vue Router configuration
│   └── index.js         # Routes and navigation guards
├── views/               # Page-level components
│   ├── Home.vue         # Connection screen (like index.html)
│   └── Wallet.vue       # Main wallet interface (like node.html)
├── components/          # Reusable UI components
│   ├── DomainCreator.vue
│   ├── TransactionSigner.vue
│   └── WalletManager.vue
├── composables/         # Business logic (like controllers)
│   ├── useWallet.js     # Wallet state management
│   └── useTransaction.js # Transaction handling
├── services/            # HTTP/API layer (like httpnode)
│   ├── api.service.js   # HTTP client to backend
│   ├── wallet.service.js
│   ├── transaction.service.js
│   └── crypto.service.js
└── utils/               # Helper functions
    ├── hash.js
    ├── storage.js
    └── validation.js
```

## Features

### Connection Management
- Connect to any peer's HTTP proxy
- Store connection in localStorage
- Disconnect and reconnect to different peers

### Wallet Operations
- Create new Ed25519 keypairs
- Load existing wallets from localStorage
- Export wallets for backup
- Display wallet ID

### Transaction Management
- Create domain registration transactions
- Generate salted domain hashes
- Sign transactions with Ed25519
- Send transactions to connected peer
- View transaction status

### Blockchain Integration
- View blockchain state
- Refresh blockchain data
- Track transaction confirmations

## Development

### Running Tests
```bash
npm test              # Run all tests
npm run test:watch    # Watch mode
npm run test:coverage # With coverage report
```

### Building for Production
```bash
npm run build
npm run preview  # Preview production build
```

### API Configuration

The frontend connects to the backend peer's HTTP proxy. The proxy address is:
- Entered by user on the Home screen
- Stored in localStorage as `proxyAddr`
- Used by all API calls in `api.service.js`

During development, Vite proxies certain endpoints (see `vite.config.js`), but in production, the frontend makes direct HTTP calls to the configured proxy address.

## Communication Flow

```
User → Home.vue (enter 127.0.0.1:8080)
         ↓
    localStorage.setItem('proxyAddr', ...)
         ↓
    Router → Wallet.vue
         ↓
    useWallet.js / useTransaction.js
         ↓
    wallet.service.js / transaction.service.js
         ↓
    api.service.js (reads proxyAddr from localStorage)
         ↓
    fetch('http://127.0.0.1:8080/namecoin/transaction')
         ↓
    Backend HTTP Proxy (gui/httpnode)
         ↓
    namecoinctrl.SubmitTransactionHandler()
         ↓
    Peer network (Paxos, Blockchain, etc.)
```

## Testing

The project includes comprehensive tests with 70%+ coverage:

- **Utils tests** - Pure functions (hash, storage, validation)
- **Services tests** - Business logic (wallet, crypto, transactions, API)
- **Composables tests** - Vue composition functions

All tests are located in `src/tests/` and follow the same structure as the source code.
