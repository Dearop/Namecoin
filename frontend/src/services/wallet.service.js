import * as ed from '@noble/ed25519';
import { bytesToHex, hexToBytes, sha256 } from '../utils/hash.js';
import { saveWalletData, getWalletData } from '../utils/storage.js';

export async function generateKeyPair() {
  const privateKey = ed.utils.randomPrivateKey();
  const publicKey = await ed.getPublicKey(privateKey);
  
  return {
    privateKey: bytesToHex(privateKey),
    publicKey: bytesToHex(publicKey)
  };
}

export async function deriveWalletID(publicKey) {
  // Use the public key as wallet ID (or hash it)
  return publicKey;
}

export function saveWallet(publicKey, privateKey) {
  const wallet = {
    publicKey,
    privateKey,
    createdAt: new Date().toISOString()
  };
  return saveWalletData(wallet);
}

export function loadWallet() {
  return getWalletData();
}

export function exportWalletToFile(wallet) {
  const dataStr = JSON.stringify(wallet, null, 2);
  const dataBlob = new Blob([dataStr], { type: 'application/json' });
  const url = URL.createObjectURL(dataBlob);
  
  const link = document.createElement('a');
  link.href = url;
  link.download = `peerster-wallet-${Date.now()}.json`;
  link.click();
  
  URL.revokeObjectURL(url);
}

export async function importWalletFromFile(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const wallet = JSON.parse(e.target.result);
        if (!wallet.publicKey || !wallet.privateKey) {
          reject(new Error('Invalid wallet file'));
        }
        resolve(wallet);
      } catch (error) {
        reject(error);
      }
    };
    reader.onerror = reject;
    reader.readAsText(file);
  });
}
