# permissions_guidelines.instructions.md
applyTo:
 - inference-chain/x/inference/keeper/msg_server_*.go
---
Message servers should, unless explained in comments otherwise, use the CheckPermission helper function found in permissions.go to determine permissions for a message as the FIRST action taken for a message.
