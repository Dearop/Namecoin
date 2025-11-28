const STORAGE_PREFIX = 'peerster_';

export const StorageKeys = {
  WALLET: `${STORAGE_PREFIX}wallet`,
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

// Wallet helpers
export function saveWalletData(wallet) {
  return setItem(StorageKeys.WALLET, wallet);
}

export function getWalletData() {
  return getItem(StorageKeys.WALLET);
}

// Domain helpers
export function saveDomain(domainData) {
  const domains = getItem(StorageKeys.DOMAINS, []);
  domains.push(domainData);
  return setItem(StorageKeys.DOMAINS, domains);
}

export function getDomains() {
  return getItem(StorageKeys.DOMAINS, []);
}