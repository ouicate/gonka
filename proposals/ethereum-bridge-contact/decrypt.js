
import { ethers } from 'ethers';
import * as fs from 'fs';
import * as readline from 'readline/promises';

// --- CONFIGURE THIS ---
const KEYSTORE_FILE_PATH = '/Users/morgachev/Library/Ethereum/keystore/UTC--2025-11-15T06-44-48.578285000Z--ce8cad52e9787b13ba41c92ad7e8dcc5921866b4';
// ----------------------

async function decryptKey() {
  console.log(`Attempting to decrypt: ${KEYSTORE_FILE_PATH}`);

  try {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout
    });

    // 1. Prompt for the password
    const password = await rl.question('Enter your geth account password: ');
    rl.close();

    // 2. Read the encrypted JSON file
    const encryptedJson = fs.readFileSync(KEYSTORE_FILE_PATH, 'utf8');

    // 3. Decrypt the wallet
    console.log('Decrypting... (this may take a moment)');
    const wallet = await ethers.Wallet.fromEncryptedJson(encryptedJson, password);

    // 4. Display the private key
    console.log('\n✅ SUCCESS!');
    console.log('Your private key is:');
    console.log(`\n${wallet.privateKey}\n`);
    console.log('🚨 DO NOT share this key with anyone!');
    console.log('Copy it, then close this window immediately.');

  } catch (error) {
    console.error('\n❌ DECRYPTION FAILED:');
    console.error(error.message);
    console.log('Check your password or file path.');
  }
}

decryptKey();