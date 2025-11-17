// Safe deployment script that works around hardhat-ethers bug
import hre from "hardhat";
import { ethers } from "ethers";
import { readFileSync } from "fs";
import { fileURLToPath } from "url";
import { dirname, join } from "path";
import dotenv from "dotenv";

dotenv.config();

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

async function main() {
    console.log("Deploying BridgeContract to Mainnet...\n");
    
    // Use raw ethers provider to avoid hardhat-ethers bug
    const provider = new ethers.JsonRpcProvider(process.env.MAINNET_RPC_URL);
    const wallet = new ethers.Wallet(process.env.PRIVATE_KEY, provider);
    
    console.log("Deploying from:", wallet.address);
    const balance = await provider.getBalance(wallet.address);
    console.log("Balance:", ethers.formatEther(balance), "ETH\n");
    
    // Get network info
    const network = await provider.getNetwork();
    console.log("Network:", network.name);
    console.log("Chain ID:", network.chainId.toString());
    
    // Define chain IDs for cross-chain replay protection
    const gonkaChainId = ethers.keccak256(ethers.toUtf8Bytes("gonka-mainnet"));
    const ethereumChainId = ethers.zeroPadValue(ethers.toBeHex(network.chainId), 32);
    
    console.log("\nChain IDs:");
    console.log("- Gonka Chain ID:", gonkaChainId);
    console.log("- Ethereum Chain ID:", ethereumChainId);
    
    // Load compiled contract
    const artifactPath = join(__dirname, "./artifacts/contracts/BridgeContract.sol/BridgeContract.json");
    const artifact = JSON.parse(readFileSync(artifactPath, "utf8"));
    
    // Create contract factory with raw ethers
    const factory = new ethers.ContractFactory(artifact.abi, artifact.bytecode, wallet);
    
    console.log("\nDeploying contract...");
    const bridge = await factory.deploy(gonkaChainId, ethereumChainId);
    
    console.log("Transaction hash:", bridge.deploymentTransaction().hash);
    console.log("Waiting for confirmations...");
    
    const receipt = await bridge.deploymentTransaction().wait(2);
    
    const contractAddress = await bridge.getAddress();
    console.log("\n✓ BridgeContract deployed successfully!");
    console.log("✓ Contract Address:", contractAddress);
    console.log("✓ Block:", receipt.blockNumber);
    console.log("✓ Gas Used:", receipt.gasUsed.toString());
    
    // Verify the initial state
    const currentState = await bridge.getCurrentState();
    const latestEpoch = await bridge.getLatestEpochInfo();
    
    console.log("\nInitial State:");
    console.log("- Contract State:", currentState === 0 ? "ADMIN_CONTROL" : "NORMAL_OPERATION");
    console.log("- Latest Epoch ID:", latestEpoch.epochId.toString());
    console.log("- Contract Owner:", await bridge.owner());
    
    console.log("\nNext Steps:");
    console.log("1. Submit genesis epoch (epoch 1) group key:");
    console.log("   npx hardhat setup-genesis --bridge", contractAddress, "--groupkey <96-byte-hex> --network mainnet");
    console.log("\n2. Verify on Etherscan (optional):");
    console.log("   npx hardhat verify --network mainnet", contractAddress, gonkaChainId, ethereumChainId);
    
    return contractAddress;
}

main()
    .then((address) => {
        console.log("\n✅ Deployment complete!");
        process.exit(0);
    })
    .catch((error) => {
        console.error("\n❌ Deployment failed:");
        console.error(error);
        process.exit(1);
    });

