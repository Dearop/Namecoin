const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

export async function sendTransaction(tx, signature) {
  try {
    //I believe the API_BASE_URL will need to be changed to the backend/node's URL
    const response = await fetch(`${API_BASE_URL}/namecoin/transaction`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        transaction: tx,
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
    const response = await fetch(`${API_BASE_URL}/namecoin/transaction/${txID}`);
    
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
    const response = await fetch(`${API_BASE_URL}/blockchain`);
    
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    console.error('[API] Get blockchain state failed:', error);
    throw error;
  }
}
