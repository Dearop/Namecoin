import { generateTxID } from '../utils/hash.js';
import canonicalize from 'canonicalize';

export async function buildTransaction(params) {
  const { type, walletID, fee, payload, pk } = params;
  
  return {
    type,
    from: walletID,
    amount: fee,
    payload: encodePayload(payload),
    pk : pk,
    transactionID: null  // Will be computed later
  };
}

export async function computeTransactionID(tx) {
  console.log('[DEBUG] computeTransactionID - input tx:', tx);
  const txID = await generateTxID({
    type: tx.type,
    from: tx.from,
    amount: tx.amount,
    payload: tx.payload,
  });
  console.log('[DEBUG] computeTransactionID - computed txID:', txID);
  
  return {
    ...tx,
    transactionID: txID
  };
}

export function encodePayload(data) {
  // Payload should be kept as-is (object or string)
  // It will be canonicalized as part of the full transaction when sending
  if (!data) {
    return {};  // Return empty object for null/undefined
  }
  // Return data as-is, whether it's a string or object
  return data;
}

export function validateTransaction(tx) {
  const errors = [];
  
  if (!tx.type) errors.push('Transaction type is required');
  if (!tx.from) errors.push('Source wallet is required');
  if (!tx.amount || tx.amount < 1) errors.push('Fee must be at least 1');
  if (!tx.payload) errors.push('Payload is required');
  if (!tx.pk) errors.push('Public key is required');
  
  return {
    valid: errors.length === 0,
    errors
  };
} 

