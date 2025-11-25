import nacl from 'tweetnacl';
import { generateSalt as _generateSalt, generateHash, sha256, bytesToHex, hexToBytes } from '../utils/hash.js';

/*
* look at hash diagram to understand the different hashing functions and their purposes
*/

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
  return await sha256(JSON.stringify(txData));
}

export async function hashTransaction(tx) {
  // Hash the entire unsigned transaction object
  // This is used for signing (step 5 in the flow)
  const txString = JSON.stringify({
    type: tx.type,
    source: tx.source,
    fee: tx.fee,
    payload: tx.payload,
    nonce: tx.nonce,
    transactionID: tx.transactionID
  });
  return await sha256(txString);
}
