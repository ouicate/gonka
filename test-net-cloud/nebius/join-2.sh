export KEY_NAME="join-2"
export PUBLIC_URL="http://89.169.110.250:8000"
export P2P_EXTERNAL_ADDRESS="tcp://89.169.110.250:5000"
export SYNC_WITH_SNAPSHOTS="true"
export DAPI_API__POC_CALLBACK_URL="http://api:9100"
#export DAPI_API__GENESIS_APP_HASH_HEX=06f565eacb705230343d1939111d5f72c547be1e58f7169f92c9c937c0a31c36
python3 launch.py --mode join --branch origin/test-upgrade-v0.2.5
