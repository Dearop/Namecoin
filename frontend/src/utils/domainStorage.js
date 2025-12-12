// Domain storage utilities for tracking pending domains

export function savePendingDomain(minerID, domain, salt, commitment, txid) {
  const key = `pending_domains_${minerID}`;
  const existing = JSON.parse(localStorage.getItem(key) || '[]');
  
  // Add new pending domain
  existing.push({
    domain,
    salt,
    commitment,
    txid,
    timestamp: Date.now(),
    status: 'pending' // pending, revealed
  });
  
  localStorage.setItem(key, JSON.stringify(existing));
}

export function getPendingDomains(minerID) {
  const key = `pending_domains_${minerID}`;
  return JSON.parse(localStorage.getItem(key) || '[]');
}

export function updateDomainStatus(minerID, domain, status) {
  const key = `pending_domains_${minerID}`;
  const existing = JSON.parse(localStorage.getItem(key) || '[]');
  
  const updated = existing.map(d => 
    d.domain === domain ? { ...d, status } : d
  );
  
  localStorage.setItem(key, JSON.stringify(updated));
}

export function removePendingDomain(minerID, domain) {
  const key = `pending_domains_${minerID}`;
  const existing = JSON.parse(localStorage.getItem(key) || '[]');
  
  const filtered = existing.filter(d => d.domain !== domain);
  
  localStorage.setItem(key, JSON.stringify(filtered));
}

export function clearAllPendingDomains(minerID) {
  const key = `pending_domains_${minerID}`;
  localStorage.removeItem(key);
}
