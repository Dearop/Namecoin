import nacl from 'tweetnacl';
import canonicalize from 'canonicalize';
import { generateSalt as _generateSalt, generateHash, sha256, bytesToHex, hexToBytes } from '../utils/hash.js';

/*
* look at hash diagram to understand the different hashing functions and their purposes
*/

// Re-export generateSalt for convenience
export { generateSalt } from '../utils/hash.js';

// Timing-safe equality comparison
function timingSafeEqual(a, b) {
  // Convert strings to Uint8Arrays if needed
  const bufA = typeof a === 'string' ? new TextEncoder().encode(a) : a;
  const bufB = typeof b === 'string' ? new TextEncoder().encode(b) : b;
  
  // Must be same length
  if (bufA.length !== bufB.length) {
    // Still do a dummy comparison to avoid timing leak on length
    let diff = 1;
    for (let i = 0; i < bufA.length; i++) {
      diff |= bufA[i] ^ 0;
    }
    return false;
  }
  
  // Constant-time comparison
  let diff = 0;
  for (let i = 0; i < bufA.length; i++) {
    diff |= bufA[i] ^ bufB[i];
  }
  
  return diff === 0;
}

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
  return timingSafeEqual(computedHash, hashedDomain);
}

export async function generateTransactionSignature(txHash, privateKey) {
  const privateKeyBytes = hexToBytes(privateKey);
  const messageBytes = hexToBytes(txHash);
  const signature = nacl.sign.detached(messageBytes, privateKeyBytes);
  return bytesToHex(signature);
}

export async function verifyTransactionSignature(txHash, signature, publicKey) {
  try {
    const publicKeyBytes = hexToBytes(publicKey);
    const messageBytes = hexToBytes(txHash);
    const signatureBytes = hexToBytes(signature);
    return nacl.sign.detached.verify(messageBytes, signatureBytes, publicKeyBytes);
  } catch (error) {
    console.error('[Crypto] Signature verification failed:', error);
    return false;
  }
}

export async function hashTransactionData(txData) {
  return await sha256(canonicalize(txData));
}

export async function hashTransaction(tx) {
    const txString = canonicalize({
        type: tx.type,
        source: tx.source,
        fee: tx.fee,
        payload: tx.payload,
        transactionID: tx.transactionID
    });
  return await sha256(txString);
}
