import canonicalize from 'canonicalize';

function getBaseURL() {
  // Get proxy address from localStorage (set in Home view)
  const proxyAddr = localStorage.getItem('proxyAddr');
  if (!proxyAddr) {
    throw new Error('No proxy address configured. Please connect to a peer first.');
  }
  return `http://${proxyAddr}`;
}

export async function sendTransaction(tx, signature) {
  try {
    const baseURL = getBaseURL();
    const response = await fetch(`${baseURL}/namecoin/new`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: canonicalize({
        type: tx.type,
        from: tx.source,
        fee: tx.fee,
        payload: tx.payload,
        publicKey: tx.source,
        txId: tx.transactionID,
        signature: signature
      })
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
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
