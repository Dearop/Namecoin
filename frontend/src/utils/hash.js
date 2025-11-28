import canonicalize from 'canonicalize';
// Browser-compatible crypto utilities using Web Crypto API

export async function sha256(input) {
  // Strict input type validation
  if (typeof input === 'string') {
    const encoder = new TextEncoder();
    const data = encoder.encode(input);
    const hashBuffer = await crypto.subtle.digest('SHA-256', data);
    const hashArray = new Uint8Array(hashBuffer);
    return bytesToHex(hashArray);
  } else if (input instanceof Uint8Array) {
    const hashBuffer = await crypto.subtle.digest('SHA-256', input);
    const hashArray = new Uint8Array(hashBuffer);
    return bytesToHex(hashArray);
  } else {
    throw new TypeError(
      `sha256: Invalid input type. Expected string or Uint8Array, got ${typeof input}. ` +
      `If you have an object or array, stringify it first using JSON.stringify() or canonicalize().`
    );
  }
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
  return await sha256(`DOMAIN_HASH_v1:${domain}:${salt}`);
}

export function generateSalt() {
  return generateRandomSalt(16);
}

export async function hashTxData(txData) {
  return await sha256(canonicalize(txData));
}

export async function generateTxID(params) {
  const { type, sourceID, fee, payload } = params;
  const payloadString = typeof payload === 'string' ? payload : canonicalize(payload);
  const txDataString = `${type}|${sourceID}|${fee}|${payloadString}`;
  return await sha256(txDataString);
}