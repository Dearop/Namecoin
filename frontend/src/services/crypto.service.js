import * as ed from '@noble/ed25519';
import { generateSalt as _generateSalt, generateHash, sha256, bytesToHex, hexToBytes } from '../utils/hash.js';

// Re-export generateSalt for convenience
export { generateSalt } from '../utils/hash.js';

export async function generateSaltedHash(domain) {
  const salt = _generateSalt();
  const hashedDomain = await generateHash(domain, salt);
  return { hashedDomain, salt };
}

export async function hashDomainWithSalt(domain, salt) {
  return await generateHash(domain, salt);
}

export async function verifyDomainHash(domain, salt, hashedDomain) {
  const computedHash = await generateHash(domain, salt);
  return computedHash === hashedDomain;
}

export async function generateTransactionSignature(txHash, privateKey) {
  const privateKeyBytes = hexToBytes(privateKey);
  const messageBytes = hexToBytes(txHash);
  const signature = await ed.sign(messageBytes, privateKeyBytes);
  return bytesToHex(signature);
}

export async function verifyTransactionSignature(txHash, signature, publicKey) {
  try {
    const publicKeyBytes = hexToBytes(publicKey);
    const messageBytes = hexToBytes(txHash);
    const signatureBytes = hexToBytes(signature);
    return await ed.verify(signatureBytes, messageBytes, publicKeyBytes);
  } catch (error) {
    console.error('[Crypto] Signature verification failed:', error);
    return false;
  }
}

export async function hashTransactionData(txData) {
  return await sha256(JSON.stringify(txData));
}

export async function hashTransaction(tx) {
  // Hash all fields except signature
  const { type, source, fee, payload, nonce } = tx;
  const txDataString = `${type}|${source}|${fee}|${payload}|${nonce}`;
  return await sha256(txDataString);
}
