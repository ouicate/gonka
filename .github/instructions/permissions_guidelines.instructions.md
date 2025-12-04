# permissions_guidelines.instructions.md
applyTo:
 - inference-chain/x/inference/keeper/msg_server_*.go
---
Message servers should, unless explained in comments otherwise, use the CheckPermission helper function found in permissions.go to determine permissions for a message as the FIRST action taken for a message.

There should be a test explicitly for the permissions for each message. The call to CheckPermission should include the permissions AND the permissions.go map should as well, so that both the implementation and the overall patterns are identifiable.

All messages should have GetSigners implemented as well to identify the signers involved. Put this in message_signers.go with all the other implementations.
