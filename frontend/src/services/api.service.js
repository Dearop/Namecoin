import canonicalize from 'canonicalize';

function getBaseURL() {
  // Use environment variable or default to localhost:8080
  const proxyAddr = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080';
  return proxyAddr;
}

export async function sendTransaction(tx, signature) {
  try {
    const baseURL = getBaseURL();
    const body = canonicalize({
      type: tx.type,
      from: tx.from,
      amount: tx.amount,
      payload: tx.payload || "",  // Ensure payload is never undefined
      pk : tx.pk,                 //public key
      txId: tx.transactionID || "",  // Ensure txId is never undefined
      signature: signature
    });
    console.log('[DEBUG] Frontend sending:', body);
    
    const response = await fetch(`${baseURL}/namecoin/new`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: body
    });

    if (!response.ok) {
      // Try to get error details from response
      const errorData = await response.json().catch(() => null);
      const errorMessage = errorData?.message || `HTTP error! status: ${response.status}`;
      throw new Error(errorMessage);
    }

    return await response.json();
  } catch (error) {
    console.error('[API] Send transaction failed:', error);
    throw error;
  }
}

export async function getTransactionStatus(txID) { //will be used later
  try {
    const baseURL = getBaseURL();
    const response = await fetch(`${baseURL}/namecoin/transaction/${txID}`);
    
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    console.error('[API] Get transaction status failed:', error);
    throw error;
  }
}

export async function getBlockchainState() {//will be used later
  try {
    const baseURL = getBaseURL();
    const response = await fetch(`${baseURL}/blockchain`);
    
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    console.error('[API] Get blockchain state failed:', error);
    throw error;
  }
}

export async function getMinerID() {
  try {
    const baseURL = getBaseURL();
    const response = await fetch(`${baseURL}/namecoin/minerid`);
    
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data = await response.json();
    return data.minerID;
  } catch (error) {
    console.error('[API] Get miner ID failed:', error);
    throw error;
  }
}
