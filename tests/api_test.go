package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cee.io/pkg/api"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/queue"
	"cee.io/pkg/utils"
)

func setupTestServer(t *testing.T, authTokens ...string) (http.Handler, func()) {
	cfg := config.Load()
	cfg.Auth.Tokens = authTokens
	cfg.Executor.Type = "process"

	execEngine, err := executor.NewExecutor(cfg)
	if err != nil {
		t.Fatalf("failed creating executor: %v", err)
	}

	q := queue.NewMemoryQueue(1000)
	wp := queue.NewWorkerPool(q, execEngine, 2)
	wp.Start()

	router := api.NewRouter(cfg, q, execEngine)

	cleanup := func() {
		wp.Stop()
		_ = q.Close()
	}

	return router, cleanup
}

func TestAPI_Health(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var res map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "healthy" {
		t.Errorf("expected healthy, got %v", res["status"])
	}
}

func TestAPI_Languages(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/languages", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var langs []map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&langs)
	if len(langs) == 0 {
		t.Errorf("expected languages list to be non-empty")
	}

	// Single language
	req2 := httptest.NewRequest(http.MethodGet, "/languages/71", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}

	// Unknown language
	req3 := httptest.NewRequest(http.MethodGet, "/languages/9999", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec3.Code)
	}
}

func TestAPI_Statuses(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/statuses", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAPI_SubmissionSyncWait(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	code := "print(10 + 25)"
	subReq := api.SubmissionRequest{
		SourceCode: &code,
		LanguageID: 71,
	}
	body, _ := json.Marshal(subReq)

	req := httptest.NewRequest(http.MethodPost, "/submissions?wait=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp api.SubmissionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Token == "" {
		t.Errorf("expected non-empty token")
	}
	if resp.Status.ID != 3 {
		t.Errorf("expected Accepted (3), got %d (%s)", resp.Status.ID, resp.Status.Description)
	}
	if resp.Stdout == nil || !strings.Contains(*resp.Stdout, "35") {
		t.Errorf("expected stdout to contain 35, got %v", resp.Stdout)
	}
}

func TestAPI_ExpectedOutput(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Matches expected output -> Accepted (3)
	code := "print('hello world')"
	expected := "hello world"
	subReq := api.SubmissionRequest{
		SourceCode:     &code,
		LanguageID:     71,
		ExpectedOutput: &expected,
	}
	body, _ := json.Marshal(subReq)

	req := httptest.NewRequest(http.MethodPost, "/submissions?wait=true", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp api.SubmissionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status.ID != 3 {
		t.Errorf("expected status 3 (Accepted), got %d", resp.Status.ID)
	}

	// 2. Mismatches expected output -> Wrong Answer (4)
	expectedMismatch := "different text"
	subReq2 := api.SubmissionRequest{
		SourceCode:     &code,
		LanguageID:     71,
		ExpectedOutput: &expectedMismatch,
	}
	body2, _ := json.Marshal(subReq2)
	req2 := httptest.NewRequest(http.MethodPost, "/submissions?wait=true", bytes.NewReader(body2))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	var resp2 api.SubmissionResponse
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2.Status.ID != 4 {
		t.Errorf("expected status 4 (Wrong Answer), got %d", resp2.Status.ID)
	}
}

func TestAPI_Base64Encoding(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	rawCode := "print('b64 test')"
	b64Code := utils.EncodeBase64(rawCode)

	subReq := api.SubmissionRequest{
		SourceCode: &b64Code,
		LanguageID: 71,
	}
	body, _ := json.Marshal(subReq)

	req := httptest.NewRequest(http.MethodPost, "/submissions?wait=true&base64_encoded=true", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp api.SubmissionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Stdout == nil {
		t.Fatalf("expected stdout not nil")
	}

	decodedStdout, err := utils.DecodeBase64(*resp.Stdout)
	if err != nil {
		t.Fatalf("failed to decode b64 stdout: %v", err)
	}
	if !strings.Contains(decodedStdout, "b64 test") {
		t.Errorf("expected decoded stdout to contain 'b64 test', got %s", decodedStdout)
	}
}

func TestAPI_BatchSubmissions(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	code1 := "print(1)"
	code2 := "print(2)"
	batchReq := api.BatchSubmissionRequest{
		Submissions: []api.SubmissionRequest{
			{SourceCode: &code1, LanguageID: 71},
			{SourceCode: &code2, LanguageID: 71},
		},
	}
	body, _ := json.Marshal(batchReq)

	req := httptest.NewRequest(http.MethodPost, "/submissions/batch", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var tokens []api.CreateTokenResponse
	_ = json.NewDecoder(rec.Body).Decode(&tokens)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	// Allow worker moment to execute
	time.Sleep(300 * time.Millisecond)

	batchGetURL := "/submissions/batch?tokens=" + tokens[0].Token + "," + tokens[1].Token
	getReq := httptest.NewRequest(http.MethodGet, batchGetURL, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}
}

func TestAPI_AuthMiddleware(t *testing.T) {
	handler, cleanup := setupTestServer(t, "secret-test-token")
	defer cleanup()

	// Request without token -> 401
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/languages", nil)
	recNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", recNoAuth.Code)
	}

	// Request with valid token -> 200
	reqAuth := httptest.NewRequest(http.MethodGet, "/languages", nil)
	reqAuth.Header.Set("X-Auth-Token", "secret-test-token")
	recAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", recAuth.Code)
	}
}

func TestCompareOutput_CRLF(t *testing.T) {
	// CRLF vs LF
	actual := "line1\r\nline2\r\nline3\r\n"
	expected := "line1\nline2\nline3\n"
	if !utils.CompareOutput(actual, expected) {
		t.Errorf("expected CRLF and LF strings to match identically")
	}

	// Trailing line spaces
	actualSpaces := "line1  \r\nline2\t\nline3\n"
	expectedSpaces := "line1\nline2\nline3"
	if !utils.CompareOutput(actualSpaces, expectedSpaces) {
		t.Errorf("expected strings with trailing line spaces to match")
	}

	// Different content should still fail
	if utils.CompareOutput("line1\r\nlineA\r\n", "line1\nlineB\n") {
		t.Errorf("expected different content to not match")
	}
}

func TestAPI_BatchValidationAtomic(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	code := "print(1)"
	invalidID := 999999 // Non-existent language ID

	batchReq := api.BatchSubmissionRequest{
		Submissions: []api.SubmissionRequest{
			{SourceCode: &code, LanguageID: 71},
			{SourceCode: &code, LanguageID: invalidID}, // should cause atomic rejection
		},
	}
	body, _ := json.Marshal(batchReq)
	req := httptest.NewRequest(http.MethodPost, "/submissions/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got %d", rec.Code)
	}
}

func TestAPI_InstallScript(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "deploy.cee.io:8080"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "deploy.cee.io:8080") {
		t.Errorf("expected script to dynamically include host header, got %s", body)
	}
	if !strings.Contains(body, "cee-") {
		t.Errorf("expected script to mention binary name, got %s", body)
	}
}

func TestAPI_DownloadBinary(t *testing.T) {
	handler, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Existing distribution binary
	req := httptest.NewRequest(http.MethodGet, "/download/cee-darwin-arm64", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cee-darwin-arm64, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("expected octet-stream, got %s", rec.Header().Get("Content-Type"))
	}

	// 2. Non-existent binary
	req404 := httptest.NewRequest(http.MethodGet, "/download/nonexistent-binary-foo", nil)
	rec404 := httptest.NewRecorder()
	handler.ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing binary, got %d", rec404.Code)
	}

	// 3. Traversal protection
	reqTrav := httptest.NewRequest(http.MethodGet, "/download/../../etc/passwd", nil)
	recTrav := httptest.NewRecorder()
	handler.ServeHTTP(recTrav, reqTrav)

	if recTrav.Code != http.StatusNotFound {
		t.Fatalf("expected 404 or blocked for traversal, got %d", recTrav.Code)
	}
}
