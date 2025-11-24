// Browser-compatible crypto utilities using Web Crypto API

export async function sha256(input) {
  const encoder = new TextEncoder();
  const data = typeof input === 'string' ? encoder.encode(input) : input;
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);
  const hashArray = new Uint8Array(hashBuffer);
  return bytesToHex(hashArray);
}

export function bytesToHex(bytes) {
  return Array.from(bytes)
    .map(b => b.toString(16).padStart(2, '0'))
    .join('');
}

export function hexToBytes(hex) {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return bytes;
}

export function generateRandomSalt(length = 32) {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytesToHex(bytes);
}

export async function generateHash(domain, salt) {
  return await sha256(domain + salt);
}

export function generateSalt() {
  return generateRandomSalt(16);
}

export async function hashTxData(txData) {
  return await sha256(JSON.stringify(txData));
}

export async function generateTxID(params) {
  const { type, sourceID, fee, payload, nonce } = params;
  const txDataString = `${type}|${sourceID}|${fee}|${payload}|${nonce}`;
  return await sha256(txDataString);
}