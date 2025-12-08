# Namecoin Frontend

A Vue 3 frontend for the Peerster Crypto Wallet that handles domain name transactions with TTL (Time-To-Live) support and a connection-based architecture similar to the backend's CLI/Web GUI system.

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

### Domain Management with TTL Support
- **NameNew (Commitment)**: Reserve domain with optional TTL preference
- **NameFirstUpdate (Reveal)**: Reveal domain with IP address and TTL
- **NameUpdate**: Update domain IP and extend TTL
- **TTL Configuration**: 
  - Default: 36,000 blocks (~4 days with 10s blocks)
  - Maximum: 5,256,000 blocks (~1 year)
  - Set to 0 to use default TTL
  - TTL resets expiration from current block height

### Transaction Management
- Create domain registration transactions
- Generate salted domain hashes (using `DOMAIN_HASH_v1:domain:salt` format)
- Sign transactions with Ed25519
- Send transactions to connected peer
- View transaction status
- Track pending commitments and domain lifecycle

### Blockchain Integration
- View blockchain state
- Refresh blockchain data
- Track transaction confirmations
- Monitor domain expiry using blockchain height

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

## Domain TTL System

### How TTL Works

**TTL (Time-To-Live)** controls how long a domain registration remains valid, measured in blocks:

1. **NameNew (Commitment Phase)**:
   - Optionally specify a TTL preference
   - If TTL=0 or omitted, no preference is stored
   - TTL preference is stored with the commitment hash

2. **NameFirstUpdate (Reveal Phase)**:
   - Can specify TTL in the transaction
   - Priority: Transaction TTL → Commitment TTL → Default (36,000 blocks)
   - Expiration = Current Block Height + Effective TTL
   - Domain added to expiry map at calculated height

3. **NameUpdate**:
   - Resets expiration from current time (not cumulative)
   - New Expiration = Current Block Height + TTL
   - Example: If domain expires at block 37,000 and you update at block 35,000 with TTL=36,000, new expiration becomes block 71,000

### Expiration Checking

- **Automatic Pruning**: Every block, expired domains are removed via `pruneExpired()`
- **Validation**: NameFirstUpdate prevents registering non-expired domains
- **Update Protection**: NameUpdate rejects updates to expired domains
- **Expiry Map**: `expires[blockHeight][]domains` tracks which domains expire at each height

## Testing

The project includes comprehensive tests with 92%+ coverage (236 tests passing):

- **Utils tests** - Pure functions (hash, storage, validation, domainStorage)
- **Services tests** - Business logic (wallet, crypto, transactions, API)
- **Composables tests** - Vue composition functions (useWallet, useTransaction)
- **Component tests** - Vue components (WalletManager, DomainCreator, TransactionSigner)
- **View tests** - Full page components (Home, Wallet)
- **Router tests** - Navigation and route guards

All tests are located in `src/tests/` and follow the same structure as the source code.
