# Build

1. Install node 22+
2. `npm install`
3. `npm run compile`

# Local deploy (hardhat testnet)

## Local Node
1. In separate terminal: `npx hardhat node`

1. Deploy:  
```
npx hardhat run deploy.js --network localhost
```
Copy <CONTRACT_ADDR> from "BridgeContract deployed to: "

2. Check state
```
HARDHAT_NETWORK=localhost node get-contract-state.js <CONTRACT_ADDR> 
```

3. Admin Submit Epoch
```
HARDHAT_NETWORK=localhost node submit-epoch.js <CONTRACT_ADDR> <epochId> <PubKey> <Signature>
```

4. Switch to non-admin state
```
HARDHAT_NETWORK=localhost node enable-normal-operation.js <CONTRACT_ADDR>
```

5. Submit non-admin epoch
```
HARDHAT_NETWORK=localhost node submit-epoch-public.js <CONTRACT_ADDR> <epochId> <PubKey> <Signature>
```


# Mainnet

## Deploy

node deploy-mainnet-safe.js

## Everything else 
```
HARDHAT_NETWORK=mainnet ....
```

# Sepolia

```
HARDHAT_NETWORK=sepolia ....
```
