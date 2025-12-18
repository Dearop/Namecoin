import { ref, computed } from 'vue';
import * as walletService from '../services/wallet.service.js';
import { clear } from '../utils/storage.js';

const wallet = ref({ publicKey: null, privateKey: null, id: null });

export function useWallet() {
  const walletID = computed(() => {
    return wallet.value.id || null;
  });

  const isWalletLoaded = computed(() => {
    return wallet.value.publicKey !== null;
  });

  async function createWallet() {
    try {
      const keypair = await walletService.generateKeyPair();
      const id = await walletService.deriveWalletID(keypair.publicKey);
      
      wallet.value = {
        publicKey: keypair.publicKey,
        privateKey: keypair.privateKey,
        id: id
      };
      
      walletService.saveWallet(keypair.publicKey, keypair.privateKey);
      
      return wallet.value;
    } catch (error) {
      console.error('[useWallet] Create wallet failed:', error);
      throw error;
    }
  }

  async function loadWallet() {
    try {
      const savedWallet = walletService.loadWallet();
      if (savedWallet) {
        const id = await walletService.deriveWalletID(savedWallet.publicKey);
        wallet.value = {
          publicKey: savedWallet.publicKey,
          privateKey: savedWallet.privateKey,
          id
        };
        return true;
      }
      return false;
    } catch (error) {
      console.error('[useWallet] Load wallet failed:', error);
      return false;
    }
  }

  function exportWallet() { //do not believe we need this, but keeping just in case
    walletService.exportWalletToFile(wallet.value);
  }

  async function importWallet(file) {//do not believe we need this, but keeping just in case
    try {
      const importedWallet = await walletService.importWalletFromFile(file);
      const id = await walletService.deriveWalletID(importedWallet.publicKey);
      wallet.value = {
        publicKey: importedWallet.publicKey,
        privateKey: importedWallet.privateKey,
        id
      };
      walletService.saveWallet(importedWallet.publicKey, importedWallet.privateKey);
      return wallet.value;
    } catch (error) {
      console.error('[useWallet] Import wallet failed:', error);
      throw error;
    }
  }

  function clearWallet() {
    wallet.value = { publicKey: null, privateKey: null, id: null };
    clear();
  }

  return {
    wallet,
    walletID,
    isWalletLoaded,
    createWallet,
    loadWallet,
    exportWallet,
    importWallet,
    clearWallet
  };
}
