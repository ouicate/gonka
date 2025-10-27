export KEY_NAME="join-2"
export PUBLIC_URL="http://89.169.110.250:8000"
export P2P_EXTERNAL_ADDRESS="tcp://89.169.110.250:5000"
export SYNC_WITH_SNAPSHOTS="true"
export DAPI_API__POC_CALLBACK_URL="http://api:9100"
python3 launch.py --mode join --branch origin/test-upgrade-v0.2.5
