# Local Setup
1. Install node 22+
2. config.env
3. 




----
HARDHAT_NETWORK=mainnet node get-contract-state.js 0x942a69BFB832a6445215A5857A7A645792295Fd9


HARDHAT_NETWORK=mainnet node submit-epoch.js "0x942a69BFB832a6445215A5857A7A645792295Fd9" 5 "uLyVx3JCSeleqDCAdj2b0+sEzNjY8u2FD02C6s3DoxULH4TT0xuHdf0Vt67drOdzBUzKR94ui9U/sO+2HuzADeUQJysmaUjYAzXPl6e4cuP+Drvu+92IL4l90/xCyqMG" "petZ+65yftYikgfnFToGqsr+gO8UBoRb2uvze/a7gBSTRFMulj72eycsZgH3VGcU"

HARDHAT_NETWORK=mainnet node submit-epoch.js "0x942a69BFB832a6445215A5857A7A645792295Fd9" 124 "qMcf/0y5ffFKOxk5PowQXCdrPWaYlz7dDJ3o3gfpTdQWF++pBnCtao9hdRulOHfvDx0UrUBhu4hjNyM75t+VMLOzm3cLC5yHs/eRO3JvHLTXGmnVE2OIz9MlZv6SKc6K" "ihmUQJoPprtTySDDUrBE172UhXEtAaEHEyRoRWNUJXzBHbcbEs6rzeU6G7k1ooAj"


node deploy-mainnet-safe.js


HARDHAT_NETWORK=mainnet node enable-normal-operation.js 0x942a69BFB832a6445215A5857A7A645792295Fd9



HARDHAT_NETWORK=mainnet node submit-epoch-public.js 0x942a69BFB832a6445215A5857A7A645792295Fd9 125 "laZUoxMOxQnoPY2CGCl9pNKqWkdDDbK0L72gNrsa3tZ1O7iCLJo4Xs4M/PIvDwCXCDOqOYUG8qPAzEQaf2KMCjFZ0Rksroq5Cr2REmAxjqXW66+JhWBeLITem46WsFQw" "jYcPY8EbOpv+d6g78++5gsZtLOGHrCRSE6t9x7qqJFavx8yeOKhZUOxGdmG1mFIu"



# Local Deployment

# 1. Deploy contract
npx hardhat run deploy.js --network localhost

BridgeContract deployed to: 0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9

# 2. Get contract state
HARDHAT_NETWORK=localhost node get-contract-state.js 0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9

# 3. Submit genesis epoch - use your actual group public key
HARDHAT_NETWORK=localhost node submit-epoch.js "0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9" 124 "qMcf/0y5ffFKOxk5PowQXCdrPWaYlz7dDJ3o3gfpTdQWF++pBnCtao9hdRulOHfvDx0UrUBhu4hjNyM75t+VMLOzm3cLC5yHs/eRO3JvHLTXGmnVE2OIz9MlZv6SKc6K" "ihmUQJoPprtTySDDUrBE172UhXEtAaEHEyRoRWNUJXzBHbcbEs6rzeU6G7k1ooAj"

# 4. Enable normal operation
HARDHAT_NETWORK=localhost node enable-normal-operation.js 0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9

# 5. Submit next epoch - use your actual keys and signatures
HARDHAT_NETWORK=localhost node submit-epoch-public.js 0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9 125 "laZUoxMOxQnoPY2CGCl9pNKqWkdDDbK0L72gNrsa3tZ1O7iCLJo4Xs4M/PIvDwCXCDOqOYUG8qPAzEQaf2KMCjFZ0Rksroq5Cr2REmAxjqXW66+JhWBeLITem46WsFQw" "jYcPY8EbOpv+d6g78++5gsZtLOGHrCRSE6t9x7qqJFavx8yeOKhZUOxGdmG1mFIu"

# Generate Test BLS Keys
# ⚠️ IMPORTANT: Keys and signatures are chain-specific!
# The transition signature includes the chain ID, so you must generate
# different keys for each network (localhost, sepolia, mainnet, etc.)

npm install @noble/curves@1.4.0

# For localhost testing (chainId 31337):
HARDHAT_NETWORK=localhost node generate-keys.js
# Creates test-keys-localhost.json

# For Sepolia testing (chainId 11155111):
HARDHAT_NETWORK=sepolia node generate-keys.js
# Creates test-keys-sepolia.json

# BridgeContract expects:
#   - Public Keys: G2 (96 bytes)
#   - Signatures: G1 (48 bytes)


----
HARDHAT_NETWORK=localhost node submit-epoch.js "0x5FbDB2315678afecb367f032d93F642f64180aa3" 1 "rNktLWsuB2KYIBf9+9mVbc9ZOTzwuPANXXEZr11npFWkdUExEdCvR/HZvH/STo1AGdnV2QEw+p4ybCg0DzMQopOhBsZNK+c8FX+S7UMgBbVguJdA3fnzMJjFBPa0lW7c" ""
HARDHAT_NETWORK=localhost node enable-normal-operation.js 0x5FbDB2315678afecb367f032d93F642f64180aa3
HARDHAT_NETWORK=localhost node submit-epoch-public.js "0x5FbDB2315678afecb367f032d93F642f64180aa3" 2 "rgwHssa1Xoi6ZOkHeAaAklHxHmJlvf8fk/7ycDNmHnIggXihJTJYgT4S8Fwbah8LBmR7fTp61VDGZI2oONhkp8/wIRpQJ2tJtAV7hw8mFuzJcr7EYMO3rReHlerlAaHq" "l0yt2Q//NRU9hhP9UhVJQ8y0cLcdB3TVtXQ8Tt+AODtK2rFuWjqvWZEULhcoXsbP"


----
# Sepolia Testing (Real BLS Verification ✅)
# Sepolia has BLS precompile support - real signature verification works!
# Contract: 0x03152c5ba93572415d3f59394802c6e1edcc3E59

# Generate Sepolia-specific keys:
HARDHAT_NETWORK=sepolia node generate-keys.js

# Deploy or use existing contract
HARDHAT_NETWORK=sepolia node deploy.js

# Submit epoch 1 (already done)
HARDHAT_NETWORK=sepolia node submit-epoch.js "0x1c3566B055f4F0ff49603152457cc88e0C1100D2" 1 "jJiocjtiDHDbUxAEkiAbPmYCMBEDGV0KpbTgSCRyIWWCEAYVX3pCmnql/YIsjvQxBjkhF/5NfU+pIKO2SNVARey5o2WBIpAQbk4Nlusw70b9DdPwgZ9Xl+UQ/IMo3sqm" ""

# Check contract state
HARDHAT_NETWORK=sepolia node get-contract-state.js 0x1c3566B055f4F0ff49603152457cc88e0C1100D2

# Enable normal operation (already done)
HARDHAT_NETWORK=sepolia node enable-normal-operation.js 0x1c3566B055f4F0ff49603152457cc88e0C1100D2

# Submit epoch 2 with Sepolia-specific signature
# ⚠️ Use keys from test-keys-sepolia.json (not test-keys-localhost.json)
HARDHAT_NETWORK=sepolia node submit-epoch-public.js "0x1c3566B055f4F0ff49603152457cc88e0C1100D2" 2 "<epoch2_pubkey>" "<sepolia_signature>"







📝 Commands for SEPOLIA Testing:

# 1. Deploy contract
HARDHAT_NETWORK=sepolia node deploy.js

# 2. Submit Epoch 1 (Genesis - no signature required)
HARDHAT_NETWORK=sepolia node submit-epoch.js "0x29553f4e550f486eB0EA96d69A713Ea630cC7231" 1 "rAQrtf6+8O1i4gfOL51NYyDAsHbXZUxMPr/dViovELZkIl3qfi9LI+IDioM7XCWmC4cBEPPB3T2IBl3OGA+vqsdo++wdCLsTWMbj2gw67RgYIabaAokxDw0zNG86nHCW" ""

# 3. Enable normal operation
HARDHAT_NETWORK=sepolia node enable-normal-operation.js 0x29553f4e550f486eB0EA96d69A713Ea630cC7231

# 4. Submit Epoch 2 (signed by Epoch 1)
HARDHAT_NETWORK=sepolia node submit-epoch-public.js "0x29553f4e550f486eB0EA96d69A713Ea630cC7231" 2 "uB6ANSeo4ETZ5HCc5dxL4G+R1gHMieC34lLQbB+lto1upvq1tNPrAMqBVxR/A4VpF4MLE9YGfZ6zCddfh+1VDL9jN5/r0J5RRfbEAgohOk7y3RLpjnY9Eo10aMcna0Lb" "ljMS3mx6bK92nNvkgplFSdQk+4xXdt8G0YhQz/Osoj0B00tgaljKmoHccfNGLvBc"