package utils

const (
	AuthorizationHeader     = "Authorization"
	XSeedHeader             = "X-Seed"
	XInferenceIdHeader      = "X-Inference-Id"
	XRequesterAddressHeader = "X-Requester-Address"
	XTimestampHeader        = "X-Timestamp"
	XTransferAddressHeader  = "X-Transfer-Address"
	XTASignatureHeader      = "X-TA-Signature"
	XPromptHashHeader       = "X-Prompt-Hash"       // Phase 3: for executor hash validation
	XValidatorAddressHeader = "X-Validator-Address" // Phase 4: for validator payload retrieval
	XEpochIdHeader          = "X-Epoch-Id"          // Phase 4: epoch for payload retrieval
)
