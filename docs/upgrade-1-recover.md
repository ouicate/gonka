
# Node Recover: Upgrade v0.2.2

The node might fail if cosmovisor was unable to download the binary even after many attempts. This procedure describes how to recover it.

Another common reason for upgrade failure is not enough space on disk for backup. You can check available disk space:

```bash
df -h .
```

If available space is less than 200GB, free up disk space or extend before proceeding.

To recover node, we need to make sure the right binaries are placed and downloaded, symlink created and restart containers.

## Step 1: Check binary hashes

Check both node and API binaries:

```bash
# Check node binary
echo "efe80be7666d58a023bccea699c23b5f31260fd274735b419950425ec6df6238 .inference/cosmovisor/current/bin/inferenced" | sudo sha256sum -c --status - && echo "Node: SUCCESS" || echo "Node: FAILED"

# Check API binary
echo "9204f04b0594f62cd00a99ce99571ed11ae8810cd6f6ac51351256b5926740c9 .dapi/cosmovisor/current/bin/decentralized-api" | sudo sha256sum -c --status - && echo "API: SUCCESS" || echo "API: FAILED"
```

If both show **SUCCESS**, no recovery is needed. If at least one shows **FAILED**, proceed with the recovery steps below.

## Step 2: Stop containers

```bash
source config.env && docker compose down node api
```

## Step 3: Recover node binary (if failed)

```bash
# Download and verify the node binary
wget https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.2-upgrade/inferenced-amd64.zip
echo "a0d8117d0bd91bd1ebe537c54668101bd60550642516a8780de301ff46d46b4b inferenced-amd64.zip" | sha256sum -c --status - && echo SUCCESS || echo FAILED

# Install and symlink
sudo rm -rf .inference/cosmovisor/upgrades/v0.2.2/bin/
sudo mkdir -p .inference/cosmovisor/upgrades/v0.2.2/bin/
unzip inferenced-amd64.zip -d temp/
sudo cp temp/inferenced .inference/cosmovisor/upgrades/v0.2.2/bin/
sudo chmod +x .inference/cosmovisor/upgrades/v0.2.2/bin/inferenced
sudo rm .inference/cosmovisor/current
sudo ln -sf upgrades/v0.2.2 .inference/cosmovisor/current
rm -rf temp/ inferenced-amd64.zip
```

## Step 4: Verify node binary recovery

```bash
# Check node binary hash
echo "efe80be7666d58a023bccea699c23b5f31260fd274735b419950425ec6df6238 .inference/cosmovisor/current/bin/inferenced" | sudo sha256sum -c --status - && echo "Node: SUCCESS" || echo "Node: FAILED"
```

## Step 5: Recover API binary (if failed)

```bash
# Download and verify the API binary
wget https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.2-upgrade/decentralized-api-amd64.zip
echo "c23f28918b28043a90e661575019ae0ad28c6b11d29a544e25bed3ff0f18caa7 decentralized-api-amd64.zip" | sha256sum -c --status - && echo SUCCESS || echo FAILED

# Install and symlink
sudo rm -rf .dapi/cosmovisor/upgrades/v0.2.2/bin/
sudo mkdir -p .dapi/cosmovisor/upgrades/v0.2.2/bin/
unzip decentralized-api-amd64.zip -d temp-api/
sudo cp temp-api/decentralized-api .dapi/cosmovisor/upgrades/v0.2.2/bin/
sudo chmod +x .dapi/cosmovisor/upgrades/v0.2.2/bin/decentralized-api
sudo rm .dapi/cosmovisor/current
sudo ln -sf upgrades/v0.2.2 .dapi/cosmovisor/current
rm -rf temp-api/ decentralized-api-amd64.zip
```

## Step 6: Verify API binary recovery

```bash
# Check API binary hash
echo "9204f04b0594f62cd00a99ce99571ed11ae8810cd6f6ac51351256b5926740c9 .dapi/cosmovisor/current/bin/decentralized-api" | sudo sha256sum -c --status - && echo "API: SUCCESS" || echo "API: FAILED"
```

## Step 7: Restart containers

```bash
source config.env && docker compose up -d node api --force-recreate
```
