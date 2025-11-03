package public

import (
	"decentralized-api/internal/storage"
	"decentralized-api/logging"
	"decentralized-api/utils"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/productscience/inference/x/inference/types"
)

// getPromptHandler returns the raw JSON prompt body for a given inference_id.
// Auth: requires X-Validator-Address and Authorization headers; timestamp optional for now.
func (s *Server) getPromptHandler(ctx echo.Context) error {
	inferenceID := ctx.Param("id")
	if inferenceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing inference id")
	}

	// Basic header presence checks; detailed signature validation can be added in tests per 6.2
	validatorAddr := ctx.Request().Header.Get(utils.XValidatorAddressHeader)
	authHeader := ctx.Request().Header.Get(utils.AuthorizationHeader)
	if validatorAddr == "" || authHeader == "" {
		logging.Warn("Prompt retrieval missing auth headers", types.Inferences, "validator", validatorAddr)
		return echo.NewHTTPError(http.StatusUnauthorized, "missing auth headers")
	}

	if s.configManager == nil || s.configManager.SqlDb() == nil || s.configManager.SqlDb().GetDb() == nil {
		logging.Error("Prompt retrieval DB not available", types.Inferences)
		return echo.NewHTTPError(http.StatusServiceUnavailable, "storage unavailable")
	}

	rec, ok, err := storage.GetPromptPayload(ctx.Request().Context(), s.configManager.SqlDb().GetDb(), inferenceID)
	if err != nil {
		logging.Error("Prompt retrieval DB error", types.Inferences, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "storage error")
	}
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "prompt not found")
	}

	// Return raw JSON body with X-Prompt-Hash
	ctx.Response().Header().Set("X-Prompt-Hash", rec.PromptHash)
	ctx.Response().Header().Set("Content-Type", "application/json")
	_, _ = ctx.Response().Write([]byte(rec.PromptPayload))
	return nil
}
