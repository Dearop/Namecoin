import { generateTxID } from '../utils/hash.js';
import { getNonce, incrementNonce } from '../utils/storage.js';

export async function buildTransaction(params) {
  const { type, walletID, fee, payload } = params;
  const nonce = params.nonce !== undefined ? params.nonce : getNonce();
  
  return {
    type,
    source: walletID,
    fee,
    payload: encodePayload(payload),
    nonce,
    transactionID: null  // Will be computed later
  };
}

export async function computeTransactionID(tx) {
  const txID = await generateTxID({
    type: tx.type,
    sourceID: tx.source,
    fee: tx.fee,
    payload: tx.payload,
    nonce: tx.nonce
  });
  
  return {
    ...tx,
    transactionID: txID
  };
}

export function encodePayload(data) {
  // Payload should be a simple string (e.g., commitment hash)
  // For name_new: just the commitment
  // For other types: could be different formats
  if (typeof data === 'string') {
    return data;
  }
  // If it's an object, stringify it (for other transaction types)
  return JSON.stringify(data);
}

export function validateTransaction(tx) {
  const errors = [];
  
  if (!tx.type) errors.push('Transaction type is required');
  if (!tx.source) errors.push('Source wallet is required');
  if (!tx.fee || tx.fee < 1) errors.push('Fee must be at least 1');
  if (!tx.payload) errors.push('Payload is required');
  if (tx.nonce === undefined) errors.push('Nonce is required');
  
  return {
    valid: errors.length === 0,
    errors
  };
} 

