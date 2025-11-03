package public

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/internal/storage"
	"decentralized-api/utils"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func setupConfigManagerWithDB(t *testing.T) *apiconfig.ConfigManager {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "test.db")
	nodeCfgPath := ""
	// minimal config file
	_ = os.WriteFile(cfgPath, []byte("nodes: []\n"), 0644)
	cm, err := apiconfig.LoadConfigManagerWithPaths(cfgPath, sqlitePath, nodeCfgPath)
	if err != nil {
		t.Fatalf("load config manager: %v", err)
	}
	return cm
}

func TestGetPrompt_Unauthorized(t *testing.T) {
	cm := setupConfigManagerWithDB(t)
	e := echo.New()
	s := &Server{e: e, configManager: cm}

	req := httptest.NewRequest(http.MethodGet, "/v1/inferences/inf-1/prompt", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/inferences/:id/prompt")
	c.SetParamNames("id")
	c.SetParamValues("inf-1")

	err := s.getPromptHandler(c)
	if err == nil {
		t.Fatalf("expected error for missing auth headers")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTPError 401, got %#v", err)
	}
}

func TestGetPrompt_Found(t *testing.T) {
	cm := setupConfigManagerWithDB(t)
	// Insert a prompt payload
	canonical, err := utils.CanonicalizeJSON([]byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	rec := storage.PromptPayloadRecord{
		InferenceID:      "inf-1",
		PromptPayload:    canonical,
		PromptHash:       utils.GenerateSHA256Hash(canonical),
		Model:            "test-model",
		RequestTimestamp: 1,
		StoredBy:         "transfer",
	}
	if err := storage.SavePromptPayload(context.Background(), cm.SqlDb().GetDb(), rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	e := echo.New()
	s := &Server{e: e, configManager: cm}

	req := httptest.NewRequest(http.MethodGet, "/v1/inferences/inf-1/prompt", nil)
	req.Header.Set(utils.XValidatorAddressHeader, "val-addr")
	req.Header.Set(utils.AuthorizationHeader, "token")
	recw := httptest.NewRecorder()
	c := e.NewContext(req, recw)
	c.SetPath("/v1/inferences/:id/prompt")
	c.SetParamNames("id")
	c.SetParamValues("inf-1")

	if err := s.getPromptHandler(c); err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if recw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recw.Code)
	}
	if got := recw.Header().Get("X-Prompt-Hash"); got != rec.PromptHash {
		t.Fatalf("expected X-Prompt-Hash %s, got %s", rec.PromptHash, got)
	}
	if recw.Body.String() != canonical+"\n" && recw.Body.String() != canonical {
		// encoder may include trailing newline or not depending on canonicalization
		t.Fatalf("unexpected body: %q", recw.Body.String())
	}
}
