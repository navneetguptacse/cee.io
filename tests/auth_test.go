package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cee.io/pkg/api"
	"cee.io/pkg/auth"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/queue"
)

func setupAuthTestServer(t *testing.T, masterAuthToken, masterMetricsToken string) (http.Handler, auth.Store, func()) {
	cfg := config.Load()
	if masterAuthToken != "" {
		cfg.Auth.Tokens = []string{masterAuthToken}
	} else {
		cfg.Auth.Tokens = nil
	}
	cfg.Executor.Type = "process"

	store := auth.NewMemoryStore()
	if err := auth.Bootstrap(t.Context(), store, cfg.Auth.Tokens, masterMetricsToken); err != nil {
		t.Fatalf("failed bootstrapping store: %v", err)
	}

	execEngine, err := executor.NewExecutor(cfg)
	if err != nil {
		t.Fatalf("failed creating executor: %v", err)
	}

	q := queue.NewMemoryQueue(100)
	router := api.NewRouter(cfg, q, execEngine, store)

	cleanup := func() {
		_ = q.Close()
		_ = store.Close()
	}

	return router, store, cleanup
}

func TestAuth_KeyGenerationAndValidation(t *testing.T) {
	// 1. Valid Auth Key Generation
	kAuth, rawAuth, err := auth.GenerateKey(auth.TypeAuth, auth.RoleGuest, "tester", "test guest key")
	if err != nil {
		t.Fatalf("unexpected error generating auth key: %v", err)
	}
	if !strings.HasPrefix(rawAuth, "cee_live_") {
		t.Errorf("expected cee_live_ prefix, got %s", rawAuth)
	}
	if len(rawAuth) < 40 {
		t.Errorf("expected sufficient key length, got %d", len(rawAuth))
	}
	if kAuth.KeyHash == "" {
		t.Errorf("expected key_hash to be populated")
	}

	// 2. Valid Metrics Key Generation
	kMetrics, rawMetrics, err := auth.GenerateKey(auth.TypeMetrics, auth.RoleMaster, "tester", "test metrics key")
	if err != nil {
		t.Fatalf("unexpected error generating metrics key: %v", err)
	}
	if !strings.HasPrefix(rawMetrics, "cee_metrics_") {
		t.Errorf("expected cee_metrics_ prefix, got %s", rawMetrics)
	}
	if kMetrics.Type != auth.TypeMetrics || kMetrics.Role != auth.RoleMaster {
		t.Errorf("expected metrics master key, got %+v", kMetrics)
	}

	// 3. Strict Rejection of Metrics + Guest
	_, _, errInvalid := auth.GenerateKey(auth.TypeMetrics, auth.RoleGuest, "tester", "invalid")
	if errInvalid == nil {
		t.Errorf("expected error for metrics+guest combination, got nil")
	}
}

func TestAuth_PermissionMatrix_Master(t *testing.T) {
	masterToken := "master-super-secret-token"
	metricsToken := "metrics-secret-token"
	handler, _, cleanup := setupAuthTestServer(t, masterToken, metricsToken)
	defer cleanup()

	// 1. Master can access normal APIs
	req := httptest.NewRequest(http.MethodGet, "/languages", nil)
	req.Header.Set("X-Auth-Token", masterToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("master should access normal APIs: expected 200, got %d", rec.Code)
	}

	// 2. Master can access metrics
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reqMetrics.Header.Set("Authorization", "Bearer "+masterToken)
	recMetrics := httptest.NewRecorder()
	handler.ServeHTTP(recMetrics, reqMetrics)
	if recMetrics.Code != http.StatusOK {
		t.Fatalf("master should access metrics: expected 200, got %d", recMetrics.Code)
	}

	// 3. Master can generate a guest key
	genGuestPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleGuest,
	})
	reqGenGuest := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genGuestPayload))
	reqGenGuest.Header.Set("X-Auth-Token", masterToken)
	recGenGuest := httptest.NewRecorder()
	handler.ServeHTTP(recGenGuest, reqGenGuest)
	if recGenGuest.Code != http.StatusCreated {
		t.Fatalf("master should generate guest key: expected 201, got %d: %s", recGenGuest.Code, recGenGuest.Body.String())
	}
	var guestResp auth.GenerateKeyResponse
	_ = json.Unmarshal(recGenGuest.Body.Bytes(), &guestResp)
	if guestResp.APIKey == "" || guestResp.Role != auth.RoleGuest {
		t.Fatalf("invalid guest key response: %+v", guestResp)
	}

	// 4. Master can generate another master key
	genMasterPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleMaster,
	})
	reqGenMaster := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genMasterPayload))
	reqGenMaster.Header.Set("X-Auth-Token", masterToken)
	recGenMaster := httptest.NewRecorder()
	handler.ServeHTTP(recGenMaster, reqGenMaster)
	if recGenMaster.Code != http.StatusCreated {
		t.Fatalf("master should generate master key: expected 201, got %d", recGenMaster.Code)
	}

	// 5. Master can generate a metrics key
	genMetricsPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeMetrics,
		Role: auth.RoleMaster,
	})
	reqGenMetrics := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genMetricsPayload))
	reqGenMetrics.Header.Set("X-Auth-Token", masterToken)
	recGenMetrics := httptest.NewRecorder()
	handler.ServeHTTP(recGenMetrics, reqGenMetrics)
	if recGenMetrics.Code != http.StatusCreated {
		t.Fatalf("master should generate metrics key: expected 201, got %d", recGenMetrics.Code)
	}

	// 6. Master can list all keys
	reqList := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	reqList.Header.Set("X-Auth-Token", masterToken)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("master should list keys: expected 200, got %d", recList.Code)
	}
	var keysList []*auth.Key
	_ = json.Unmarshal(recList.Body.Bytes(), &keysList)
	if len(keysList) < 4 { // bootstrap auth + guest + master + metrics
		t.Fatalf("expected at least 4 keys in list, got %d", len(keysList))
	}

	// 7. Master capabilities endpoint
	reqCap := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	reqCap.Header.Set("X-Auth-Token", masterToken)
	recCap := httptest.NewRecorder()
	handler.ServeHTTP(recCap, reqCap)
	if recCap.Code != http.StatusOK {
		t.Fatalf("master capabilities: expected 200, got %d", recCap.Code)
	}
	var capInfo auth.CapabilityInfo
	_ = json.Unmarshal(recCap.Body.Bytes(), &capInfo)
	if capInfo.Role != auth.RoleMaster || capInfo.CredentialType != auth.TypeAuth {
		t.Fatalf("unexpected capabilities: %+v", capInfo)
	}
}

func TestAuth_PermissionMatrix_Guest(t *testing.T) {
	masterToken := "master-token"
	handler, _, cleanup := setupAuthTestServer(t, masterToken, "")
	defer cleanup()

	// 1. Generate a guest key using the master key
	genGuestPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleGuest,
	})
	reqGen := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genGuestPayload))
	reqGen.Header.Set("X-Auth-Token", masterToken)
	recGen := httptest.NewRecorder()
	handler.ServeHTTP(recGen, reqGen)
	if recGen.Code != http.StatusCreated {
		t.Fatalf("failed generating guest key: %d", recGen.Code)
	}
	var guestResp auth.GenerateKeyResponse
	_ = json.Unmarshal(recGen.Body.Bytes(), &guestResp)
	guestToken := guestResp.APIKey

	// 2. Guest CAN access normal application APIs
	reqApp := httptest.NewRequest(http.MethodGet, "/about", nil)
	reqApp.Header.Set("X-Auth-Token", guestToken)
	recApp := httptest.NewRecorder()
	handler.ServeHTTP(recApp, reqApp)
	if recApp.Code != http.StatusOK {
		t.Fatalf("guest should access normal APIs: expected 200, got %d", recApp.Code)
	}

	// 3. Guest CAN generate another guest key (Delegation)
	delPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleGuest,
	})
	reqDel := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(delPayload))
	reqDel.Header.Set("X-Auth-Token", guestToken)
	recDel := httptest.NewRecorder()
	handler.ServeHTTP(recDel, reqDel)
	if recDel.Code != http.StatusCreated {
		t.Fatalf("guest should be allowed to delegate/generate another guest key: expected 201, got %d", recDel.Code)
	}

	// 4. Guest CANNOT generate a master key (Privilege escalation blocked)
	escPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleMaster,
	})
	reqEsc := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(escPayload))
	reqEsc.Header.Set("X-Auth-Token", guestToken)
	recEsc := httptest.NewRecorder()
	handler.ServeHTTP(recEsc, reqEsc)
	if recEsc.Code != http.StatusForbidden {
		t.Fatalf("guest must NOT be allowed to generate master key: expected 403, got %d", recEsc.Code)
	}

	// 5. Guest CANNOT generate a metrics key
	metPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeMetrics,
		Role: auth.RoleMaster,
	})
	reqMet := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(metPayload))
	reqMet.Header.Set("X-Auth-Token", guestToken)
	recMet := httptest.NewRecorder()
	handler.ServeHTTP(recMet, reqMet)
	if recMet.Code != http.StatusForbidden {
		t.Fatalf("guest must NOT be allowed to generate metrics key: expected 403, got %d", recMet.Code)
	}

	// 6. Guest CANNOT access metrics (/metrics)
	reqGetMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reqGetMetrics.Header.Set("X-Auth-Token", guestToken)
	recGetMetrics := httptest.NewRecorder()
	handler.ServeHTTP(recGetMetrics, reqGetMetrics)
	if recGetMetrics.Code != http.StatusForbidden {
		t.Fatalf("guest must NOT be allowed to access /metrics: expected 403, got %d", recGetMetrics.Code)
	}

	// 7. Guest CANNOT list keys
	reqList := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	reqList.Header.Set("X-Auth-Token", guestToken)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusForbidden {
		t.Fatalf("guest must NOT be allowed to list keys: expected 403, got %d", recList.Code)
	}

	// 8. Guest CANNOT revoke any key
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/api/keys/"+guestResp.ID, nil)
	reqRevoke.Header.Set("X-Auth-Token", guestToken)
	recRevoke := httptest.NewRecorder()
	handler.ServeHTTP(recRevoke, reqRevoke)
	if recRevoke.Code != http.StatusForbidden {
		t.Fatalf("guest must NOT be allowed to revoke keys: expected 403, got %d", recRevoke.Code)
	}
}

func TestAuth_PermissionMatrix_Metrics(t *testing.T) {
	masterToken := "master-token"
	metricsToken := "metrics-token"
	handler, _, cleanup := setupAuthTestServer(t, masterToken, metricsToken)
	defer cleanup()

	// 1. Metrics Master CAN access /metrics
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reqMetrics.Header.Set("X-Metrics-Token", metricsToken)
	recMetrics := httptest.NewRecorder()
	handler.ServeHTTP(recMetrics, reqMetrics)
	if recMetrics.Code != http.StatusOK {
		t.Fatalf("metrics master should access /metrics: expected 200, got %d", recMetrics.Code)
	}

	// 2. Metrics Master CANNOT access normal application APIs
	reqNormal := httptest.NewRequest(http.MethodGet, "/languages", nil)
	reqNormal.Header.Set("X-Auth-Token", metricsToken)
	recNormal := httptest.NewRecorder()
	handler.ServeHTTP(recNormal, reqNormal)
	if recNormal.Code != http.StatusForbidden {
		t.Fatalf("metrics key should NOT access normal application APIs: expected 403, got %d", recNormal.Code)
	}

	// 3. Metrics Master CANNOT generate keys
	genPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleGuest,
	})
	reqGen := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genPayload))
	reqGen.Header.Set("X-Auth-Token", metricsToken)
	recGen := httptest.NewRecorder()
	handler.ServeHTTP(recGen, reqGen)
	if recGen.Code != http.StatusForbidden {
		t.Fatalf("metrics key should NOT generate keys: expected 403, got %d", recGen.Code)
	}
}

func TestAuth_LockoutProtection_LastMasterKey(t *testing.T) {
	masterToken := "only-master-token"
	handler, store, cleanup := setupAuthTestServer(t, masterToken, "")
	defer cleanup()

	// Count master keys -> 1
	count, _ := store.CountActiveMasterAuthKeys(t.Context())
	if count != 1 {
		t.Fatalf("expected 1 active master key, got %d", count)
	}

	// 1. Attempt to revoke the only master key -> 409 Conflict
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/api/keys/key_bootstrap_auth_1", nil)
	reqRevoke.Header.Set("X-Auth-Token", masterToken)
	recRevoke := httptest.NewRecorder()
	handler.ServeHTTP(recRevoke, reqRevoke)
	if recRevoke.Code != http.StatusConflict {
		t.Fatalf("revoking last master key must be rejected with 409 Conflict, got %d: %s", recRevoke.Code, recRevoke.Body.String())
	}

	// 2. Generate a second master key
	genPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleMaster,
	})
	reqGen := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genPayload))
	reqGen.Header.Set("X-Auth-Token", masterToken)
	recGen := httptest.NewRecorder()
	handler.ServeHTTP(recGen, reqGen)
	if recGen.Code != http.StatusCreated {
		t.Fatalf("failed generating second master key: %d", recGen.Code)
	}
	var secondMasterResp auth.GenerateKeyResponse
	_ = json.Unmarshal(recGen.Body.Bytes(), &secondMasterResp)

	// Now 2 active master keys exist
	count2, _ := store.CountActiveMasterAuthKeys(t.Context())
	if count2 != 2 {
		t.Fatalf("expected 2 active master keys, got %d", count2)
	}

	// 3. Now revoking the first master key SUCCEEDS
	reqRevokeSuccess := httptest.NewRequest(http.MethodDelete, "/api/keys/key_bootstrap_auth_1", nil)
	reqRevokeSuccess.Header.Set("X-Auth-Token", secondMasterResp.APIKey)
	recRevokeSuccess := httptest.NewRecorder()
	handler.ServeHTTP(recRevokeSuccess, reqRevokeSuccess)
	if recRevokeSuccess.Code != http.StatusOK {
		t.Fatalf("revoking first master should succeed when second master exists: expected 200, got %d", recRevokeSuccess.Code)
	}

	// 4. Now the first master key is REVOKED -> requests using it must fail with 401
	reqRevokedAuth := httptest.NewRequest(http.MethodGet, "/about", nil)
	reqRevokedAuth.Header.Set("X-Auth-Token", masterToken)
	recRevokedAuth := httptest.NewRecorder()
	handler.ServeHTTP(recRevokedAuth, reqRevokedAuth)
	if recRevokedAuth.Code != http.StatusUnauthorized {
		t.Fatalf("revoked master key must immediately return 401, got %d", recRevokedAuth.Code)
	}

	// 5. Attempting to revoke the second master key (now the last one remaining) must fail with 409
	reqRevokeLastAgain := httptest.NewRequest(http.MethodDelete, "/api/keys/"+secondMasterResp.ID, nil)
	reqRevokeLastAgain.Header.Set("X-Auth-Token", secondMasterResp.APIKey)
	recRevokeLastAgain := httptest.NewRecorder()
	handler.ServeHTTP(recRevokeLastAgain, reqRevokeLastAgain)
	if recRevokeLastAgain.Code != http.StatusConflict {
		t.Fatalf("revoking remaining last master must fail with 409 Conflict, got %d", recRevokeLastAgain.Code)
	}
}

func TestAuth_API_RoleValidationAndNonLeakage(t *testing.T) {
	masterToken := "master-token"
	handler, _, cleanup := setupAuthTestServer(t, masterToken, "")
	defer cleanup()

	// 1. Generate a guest key
	genGuestPayload, _ := json.Marshal(auth.GenerateKeyRequest{
		Type: auth.TypeAuth,
		Role: auth.RoleGuest,
	})
	reqGen := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(genGuestPayload))
	reqGen.Header.Set("X-Auth-Token", masterToken)
	recGen := httptest.NewRecorder()
	handler.ServeHTTP(recGen, reqGen)
	if recGen.Code != http.StatusCreated {
		t.Fatalf("failed generating guest key: %d", recGen.Code)
	}
	var guestResp auth.GenerateKeyResponse
	_ = json.Unmarshal(recGen.Body.Bytes(), &guestResp)
	guestToken := guestResp.APIKey

	// 2. Query capabilities with expected role:
	// A) Master key checking for role=guest -> MUST fail with 403 and NOT leak that it is a master key
	reqCapMismatch1 := httptest.NewRequest(http.MethodGet, "/api/capabilities?role=guest", nil)
	reqCapMismatch1.Header.Set("X-Auth-Token", masterToken)
	recCapMismatch1 := httptest.NewRecorder()
	handler.ServeHTTP(recCapMismatch1, reqCapMismatch1)
	if recCapMismatch1.Code != http.StatusForbidden {
		t.Fatalf("master key with role=guest must be forbidden: expected 403, got %d", recCapMismatch1.Code)
	}
	if strings.Contains(recCapMismatch1.Body.String(), "MASTER") {
		t.Fatalf("error message must NOT leak key owner (MASTER): %s", recCapMismatch1.Body.String())
	}
	if !strings.Contains(recCapMismatch1.Body.String(), "Invalid or unauthorized API key") {
		t.Fatalf("expected 'Invalid or unauthorized API key', got: %s", recCapMismatch1.Body.String())
	}

	// B) Guest key checking for role=master -> MUST fail with 403 and NOT leak that it is a guest key
	reqCapMismatch2 := httptest.NewRequest(http.MethodGet, "/api/capabilities?role=master", nil)
	reqCapMismatch2.Header.Set("X-Auth-Token", guestToken)
	recCapMismatch2 := httptest.NewRecorder()
	handler.ServeHTTP(recCapMismatch2, reqCapMismatch2)
	if recCapMismatch2.Code != http.StatusForbidden {
		t.Fatalf("guest key with role=master must be forbidden: expected 403, got %d", recCapMismatch2.Code)
	}
	if strings.Contains(recCapMismatch2.Body.String(), "GUEST") {
		t.Fatalf("error message must NOT leak key owner (GUEST): %s", recCapMismatch2.Body.String())
	}
	if !strings.Contains(recCapMismatch2.Body.String(), "Invalid or unauthorized API key") {
		t.Fatalf("expected 'Invalid or unauthorized API key', got: %s", recCapMismatch2.Body.String())
	}

	// 3. Test POST /api/auth/login API endpoint:
	// A) Guest key logging in as guest -> success
	loginGuestPayload, _ := json.Marshal(map[string]string{"role": "guest"})
	reqLoginGuest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginGuestPayload))
	reqLoginGuest.Header.Set("X-Auth-Token", guestToken)
	recLoginGuest := httptest.NewRecorder()
	handler.ServeHTTP(recLoginGuest, reqLoginGuest)
	if recLoginGuest.Code != http.StatusOK {
		t.Fatalf("guest login: expected 200, got %d: %s", recLoginGuest.Code, recLoginGuest.Body.String())
	}
	if !strings.Contains(recLoginGuest.Body.String(), "Successfully logged in!") {
		t.Fatalf("expected 'Successfully logged in!' in response: %s", recLoginGuest.Body.String())
	}

	// B) Master key logging in as guest -> MUST return 403 Forbidden without leaking role
	reqLoginMismatch := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginGuestPayload))
	reqLoginMismatch.Header.Set("X-Auth-Token", masterToken)
	recLoginMismatch := httptest.NewRecorder()
	handler.ServeHTTP(recLoginMismatch, reqLoginMismatch)
	if recLoginMismatch.Code != http.StatusForbidden {
		t.Fatalf("master key logging in as guest must be 403: got %d", recLoginMismatch.Code)
	}
	if strings.Contains(recLoginMismatch.Body.String(), "MASTER") {
		t.Fatalf("error response must NOT leak MASTER: %s", recLoginMismatch.Body.String())
	}
	if !strings.Contains(recLoginMismatch.Body.String(), "Invalid or unauthorized API key") {
		t.Fatalf("expected 'Invalid or unauthorized API key', got: %s", recLoginMismatch.Body.String())
	}

	// C) Master key logging in as master -> success
	loginMasterPayload, _ := json.Marshal(map[string]string{"role": "master"})
	reqLoginMaster := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginMasterPayload))
	reqLoginMaster.Header.Set("X-Auth-Token", masterToken)
	recLoginMaster := httptest.NewRecorder()
	handler.ServeHTTP(recLoginMaster, reqLoginMaster)
	if recLoginMaster.Code != http.StatusOK {
		t.Fatalf("master login: expected 200, got %d: %s", recLoginMaster.Code, recLoginMaster.Body.String())
	}
	if !strings.Contains(recLoginMaster.Body.String(), "Successfully logged in!") {
		t.Fatalf("expected 'Successfully logged in!' in response: %s", recLoginMaster.Body.String())
	}

	// 4. Test POST /api/auth/logout API endpoint:
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	reqLogout.Header.Set("X-Auth-Token", masterToken)
	recLogout := httptest.NewRecorder()
	handler.ServeHTTP(recLogout, reqLogout)
	if recLogout.Code != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d: %s", recLogout.Code, recLogout.Body.String())
	}
	if !strings.Contains(recLogout.Body.String(), "Successfully logged out!") {
		t.Fatalf("expected 'Successfully logged out!' in response: %s", recLogout.Body.String())
	}
}
