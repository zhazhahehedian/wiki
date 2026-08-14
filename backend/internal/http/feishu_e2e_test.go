package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	configstore "github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

// TestFeishuOAuthImportSyncOwnershipE2E stops HTTP assertions at the queue
// contract. Explicit drains below represent the in-memory worker boundary.
func TestFeishuOAuthImportSyncOwnershipE2E(t *testing.T) {
	harness := newFeishuE2EHarness(t)
	owner := harness.login(t, "owner")
	other := harness.login(t, "other")

	if owner.sessionCookie == nil || owner.csrfCookie == nil {
		t.Fatal("OAuth callback did not issue the session and CSRF cookies")
	}
	if !owner.sessionCookie.HttpOnly || !owner.sessionCookie.Secure || owner.csrfCookie.HttpOnly || !owner.csrfCookie.Secure || owner.sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("auth cookie flags = session:%#v csrf:%#v", owner.sessionCookie, owner.csrfCookie)
	}

	beforeQueue, beforeSource := harness.queueCalls, harness.sourceCalls
	response := harness.mutate(t, owner, http.MethodPost, "/api/v1/kbs/"+harness.ownerKBID+"/feishu-imports", `{"url":"https://acme.feishu.cn/docx/e2eDoc"}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("import status = %d, body = %s", response.Code, response.Body.String())
	}
	var imported domain.Document
	if err := json.NewDecoder(response.Body).Decode(&imported); err != nil {
		t.Fatalf("decode imported document: %v", err)
	}
	if imported.ID == "" || imported.KBID != harness.ownerKBID || imported.SourceType != "feishu-docx" {
		t.Fatalf("imported document = %#v", imported)
	}
	if harness.queueCalls != beforeQueue+1 || harness.sourceCalls != beforeSource {
		t.Fatalf("import must only enqueue before 202: queue/source = %d/%d, before %d/%d", harness.queueCalls, harness.sourceCalls, beforeQueue, beforeSource)
	}
	harness.assertNextQueuedSync(t, imported.ID, service.InitialFeishuRevision)
	harness.drainNextSync(t)
	harness.assertLoadedSnapshot(t, imported.ID)
	harness.assertNoSecrets(t, response.Header().Get("Location")+response.Body.String())

	beforeQueue, beforeSource = harness.queueCalls, harness.sourceCalls
	response = harness.mutate(t, owner, http.MethodPost, "/api/v1/docs/"+imported.ID+"/sync", `{}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("manual sync status = %d, body = %s", response.Code, response.Body.String())
	}
	if harness.queueCalls != beforeQueue+1 || harness.sourceCalls != beforeSource {
		t.Fatalf("manual sync must only enqueue before 202: queue/source = %d/%d, before %d/%d", harness.queueCalls, harness.sourceCalls, beforeQueue, beforeSource)
	}
	harness.assertNextQueuedSync(t, imported.ID, "7")
	harness.drainNextSync(t)
	if harness.sourceCalls <= beforeSource {
		t.Fatalf("explicit sync drain did not access fake Feishu API: source calls = %d, before %d", harness.sourceCalls, beforeSource)
	}
	harness.assertLoadedSnapshot(t, imported.ID)

	beforeQueue, beforeSource = harness.queueCalls, harness.sourceCalls
	response = harness.mutate(t, other, http.MethodPost, "/api/v1/kbs/"+harness.ownerKBID+"/feishu-imports", `{"url":"https://acme.feishu.cn/docx/otherDoc"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner import status = %d, body = %s", response.Code, response.Body.String())
	}
	response = harness.mutate(t, other, http.MethodPost, "/api/v1/docs/"+imported.ID+"/sync", `{}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner sync status = %d, body = %s", response.Code, response.Body.String())
	}
	if harness.queueCalls != beforeQueue || harness.sourceCalls != beforeSource {
		t.Fatalf("rejected cross-owner requests reached queue/source: %d/%d, want %d/%d", harness.queueCalls, harness.sourceCalls, beforeQueue, beforeSource)
	}

	if harness.oauthAuthorizeCalls != 2 || harness.oauthTokenCalls != 2 || harness.oauthUserInfoCalls != 2 {
		t.Fatalf("fake OAuth calls = authorize:%d token:%d user_info:%d", harness.oauthAuthorizeCalls, harness.oauthTokenCalls, harness.oauthUserInfoCalls)
	}
	harness.assertNoSecrets(t, harness.logs.String())
	if strings.Contains(harness.logs.String(), "state=") || strings.Contains(harness.logs.String(), "code=") {
		t.Fatalf("access log contains OAuth callback query: %s", harness.logs.String())
	}
}

func TestFeishuOperatorDocumentationMatchesRuntimeContracts(t *testing.T) {
	configSource := readFeishuE2EFile(t, "../config/config.go")
	authSource := readFeishuE2EFile(t, "../../cmd/server/auth.go")
	envExample := readFeishuE2EFile(t, "../../../.env.example")
	guide := readFeishuE2EFile(t, "../../../docs/deploy-debug-feishu.md")
	readme := readFeishuE2EFile(t, "../../../README.md")
	claude := readFeishuE2EFile(t, "../../../CLAUDE.md")
	feishuClientSource := readFeishuE2EFile(t, "../infra/feishu/client.go")
	feishuOAuthSource := readFeishuE2EFile(t, "../infra/feishu/oauth_client.go")
	bitableSource := readFeishuE2EFile(t, "../infra/feishu/bitable_loader.go")
	runtimeEnv := runtimeFeishuEnvironment(t, configSource)
	runtimeScopes := runtimeRequiredFeishuScopes(t, authSource)

	environmentVariables := []string{
		"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_REDIRECT_URL", "FEISHU_TENANT_KEY",
		"OAUTH_ENCRYPTION_KEY", "SESSION_COOKIE_SECURE", "SESSION_TTL", "FRONTEND_ORIGIN",
		"BOOTSTRAP_OWNER_FEISHU_OPEN_ID", "FEISHU_SYNC_JOB_TIMEOUT", "INGESTION_JOB_TIMEOUT",
		"FEISHU_RECONCILE_JOB_TIMEOUT", "FEISHU_RECONCILE_INTERVAL", "FEISHU_SYNC_LEASE",
		"RIVER_RESCUE_STUCK_JOBS_AFTER", "FEISHU_RECONCILE_BATCH_SIZE", "FEISHU_RECONCILE_MAX_BATCHES",
	}
	expectedEnv := stringSet(environmentVariables)
	assertFeishuE2ESetEqual(t, "runtime env", runtimeEnv, expectedEnv)
	assertFeishuE2ESetEqual(t, ".env.example env", documentedFeishuEnvFromTemplate(envExample), expectedEnv)
	assertFeishuE2ESetEqual(t, "deployment guide env", documentedFeishuEnvFromGuide(t, guide), expectedEnv)
	for _, name := range environmentVariables {
		if !strings.Contains(guide, "`"+name+"`") {
			t.Errorf("deployment guide does not explain %s", name)
		}
	}
	publicKey := envTemplateValue(t, envExample, "OAUTH_ENCRYPTION_KEY")
	if publicKey != "" && slices.Contains([]int{16, 24, 32}, len([]byte(publicKey))) {
		t.Fatalf(".env.example provides a runtime-acceptable public OAuth encryption key of %d bytes", len([]byte(publicKey)))
	}
	applyEnvironmentTemplate(t, envExample)
	if _, err := configstore.Load(); err == nil || !strings.Contains(err.Error(), "missing required env var: OAUTH_ENCRYPTION_KEY") {
		t.Fatalf("config.Load with unchanged template error = %v, want missing OAUTH_ENCRYPTION_KEY", err)
	}
	for _, readmeContract := range []string{"openssl rand -hex 16", "不可使用模板值"} {
		if !strings.Contains(readme, readmeContract) {
			t.Errorf("README is missing OAuth key contract %q", readmeContract)
		}
	}
	firstMakeUp := strings.Index(readme, "make up")
	if firstMakeUp < 0 {
		t.Fatal("README does not contain the Quick Start make up command")
	}
	quickStartBeforeMakeUp := readme[:firstMakeUp]
	for _, prerequisite := range []string{
		"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_REDIRECT_URL",
		"OAUTH_ENCRYPTION_KEY", "FRONTEND_ORIGIN", "openssl rand -hex 16",
	} {
		if !strings.Contains(quickStartBeforeMakeUp, prerequisite) {
			t.Errorf("README must require %q before the first make up", prerequisite)
		}
	}
	if strings.Contains(readme, "阶段 0 还用不上") {
		t.Error("README still says required startup keys are unnecessary in phase 0")
	}
	if !slices.Equal(runtimeScopes, feishuE2ERequiredScopes()) {
		t.Fatalf("runtime requiredFeishuScopes = %#v, E2E contract = %#v", runtimeScopes, feishuE2ERequiredScopes())
	}
	for _, scope := range runtimeScopes {
		if !strings.Contains(guide, "`"+scope+"`") {
			t.Errorf("deployment guide does not document scope %s", scope)
		}
	}
	for _, contract := range []string{
		"oauth_state_invalid", "tenant_not_allowed", "feishu_reauth_required", "csrf_rejected",
		"HttpOnly", "SameSite=Lax", "60", "reconciler", "MinIO", "NULL owner", "后端不直接读取 `it_wiki_csrf` cookie",
	} {
		if !strings.Contains(guide, contract) {
			t.Errorf("deployment guide is missing contract %q", contract)
		}
	}
	runtimeDefaults := []struct {
		source, name, expression, documented string
	}{
		{feishuOAuthSource, "defaultBaseURL", `"https://open.feishu.cn"`, "`https://open.feishu.cn`"},
		{feishuClientSource, "defaultRequestTimeout", "15 * time.Second", "15s timeout"},
		{feishuClientSource, "defaultMaxRetries", "3", "最多 3 次重试"},
		{feishuClientSource, "defaultMaxRetryDelay", "30 * time.Second", "退避上限 30s"},
		{feishuClientSource, "maxAPIResponseBytes", "8 << 20", "单响应 8 MiB"},
		{feishuClientSource, "defaultMaxRows", "10_000", "最多 10,000 行"},
		{feishuClientSource, "defaultMaxOutputBytes", "10 << 20", "10 MiB canonical output"},
		{bitableSource, "defaultBitableMaxRows", "10_000", "最多 10,000 行"},
		{bitableSource, "defaultBitableMaxOutputBytes", "10 << 20", "10 MiB canonical output"},
	}
	for _, contract := range runtimeDefaults {
		if got := runtimeConstantExpression(t, contract.source, contract.name); got != contract.expression {
			t.Errorf("runtime constant %s = %q, want %q", contract.name, got, contract.expression)
		}
		if !strings.Contains(guide, contract.documented) {
			t.Errorf("deployment guide is missing runtime default %q", contract.documented)
		}
	}
	if !strings.Contains(readme, "docs/deploy-debug-feishu.md") {
		t.Error("README does not link the Feishu deployment/debugging guide")
	}
	for _, contract := range []string{"BOOTSTRAP_OWNER_FEISHU_OPEN_ID", "FEISHU_REDIRECT_URL", "feishu-imports", "docs/deploy-debug-feishu.md"} {
		if !strings.Contains(claude, contract) {
			t.Errorf("CLAUDE.md is missing Feishu workflow contract %q", contract)
		}
	}
}

type feishuE2ESession struct {
	sessionCookie *http.Cookie
	csrfCookie    *http.Cookie
}

type feishuE2EHarness struct {
	ownerKBID                           string
	queueCalls, sourceCalls             int
	oauthAuthorizeCalls                 int
	oauthTokenCalls, oauthUserInfoCalls int
	logs                                bytes.Buffer
	router                              http.Handler
	store                               *feishuE2EStore
	oauthServer                         *httptest.Server
	apiServer                           *httptest.Server
	queue                               *feishuE2EQueue
	pendingIdentity                     string
	authorizedCodes                     map[string]string
	accessTokens                        map[string]string
	oauthStates                         []string
	snapshots                           map[string]feishuE2ESnapshot
}

func newFeishuE2EHarness(t *testing.T) *feishuE2EHarness {
	t.Helper()
	h := &feishuE2EHarness{
		ownerKBID:       uuid.NewString(),
		authorizedCodes: make(map[string]string),
		accessTokens:    make(map[string]string),
		snapshots:       make(map[string]feishuE2ESnapshot),
	}

	previousLogWriter := log.Writer()
	log.SetOutput(&h.logs)
	t.Cleanup(func() { log.SetOutput(previousLogWriter) })

	h.oauthServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serveFakeOAuth(t, w, r)
	}))
	t.Cleanup(h.oauthServer.Close)
	h.apiServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serveFakeFeishuAPI(t, w, r)
	}))
	t.Cleanup(h.apiServer.Close)

	store := newFeishuE2EStore()
	h.store = store
	protector, err := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	sessions := authstore.NewMemorySessionStore(nil)
	authService := service.NewAuth(service.AuthConfig{
		TenantKey: "tenant-e2e", RequiredScopes: feishuE2ERequiredScopes(), SessionTTL: time.Hour,
	}, feishu.NewOAuthClient(feishu.OAuthConfig{
		BaseURL: h.oauthServer.URL, AppID: "cli_e2e", AppSecret: "e2e-app-secret-canary",
		RedirectURL: "https://backend.example.test/api/v1/auth/feishu/callback",
	}, h.oauthServer.Client()), protector, sessions, authstore.NewMemoryOAuthStateStore(nil), store)
	authHandler, err := NewAuthHandler(AuthHandlerConfig{
		AuthorizeURL: h.oauthServer.URL + "/open-apis/authen/v1/authorize",
		AppID:        "cli_e2e", RedirectURL: "https://backend.example.test/api/v1/auth/feishu/callback",
		FrontendOrigin: "https://app.example.test", FrontendPath: "/", CookieSecure: true, SessionTTL: time.Hour,
	}, authService, sessions, store)
	if err != nil {
		t.Fatal(err)
	}

	loader := feishu.NewDocxLoader(feishu.NewClient(feishu.ClientConfig{
		BaseURL: h.apiServer.URL, MaxRetries: -1,
	}, h.apiServer.Client()))
	queue := &feishuE2EQueue{harness: h, store: store, auth: authService, resolver: feishu.NewURLResolver(), loader: loader}
	h.queue = queue
	feishuHandler := NewFeishuHandler(
		service.NewFeishuAccounts(store),
		service.NewFeishuImport(feishu.NewURLResolver(), store, queue),
		service.NewFeishuSync(store, queue),
	)
	h.router = NewRouter(Handlers{Auth: authHandler, Feishu: feishuHandler})
	return h
}

func (h *feishuE2EHarness) login(t *testing.T, identity string) feishuE2ESession {
	t.Helper()
	h.pendingIdentity = identity
	start := httptest.NewRecorder()
	h.router.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/start", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("OAuth start status = %d, body = %s", start.Code, start.Body.String())
	}
	stateCookie := findE2ECookie(start.Result().Cookies(), OAuthStateCookieName)
	if stateCookie == nil || !stateCookie.HttpOnly || !stateCookie.Secure || stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("OAuth state cookie = %#v", stateCookie)
	}
	authorizeURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil || authorizeURL.Host != strings.TrimPrefix(h.oauthServer.URL, "https://") {
		t.Fatalf("OAuth authorize URL = %q, err = %v", start.Header().Get("Location"), err)
	}
	if authorizeURL.Query().Get("state") == "" || authorizeURL.Query().Get("state") != stateCookie.Value {
		t.Fatalf("OAuth state query/cookie mismatch")
	}

	client := h.oauthServer.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	authorizeResponse, err := client.Get(authorizeURL.String())
	if err != nil {
		t.Fatalf("call fake OAuth authorize endpoint: %v", err)
	}
	defer authorizeResponse.Body.Close()
	if authorizeResponse.StatusCode != http.StatusFound {
		t.Fatalf("fake authorize status = %d", authorizeResponse.StatusCode)
	}
	callbackURL, err := url.Parse(authorizeResponse.Header.Get("Location"))
	if err != nil || callbackURL.Host != "backend.example.test" {
		t.Fatalf("OAuth callback URL = %q, err = %v", authorizeResponse.Header.Get("Location"), err)
	}
	callback := httptest.NewRequest(http.MethodGet, callbackURL.RequestURI(), nil)
	callback.AddCookie(stateCookie)
	callbackResponse := httptest.NewRecorder()
	h.router.ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != "https://app.example.test/" {
		t.Fatalf("OAuth callback status/location = %d/%q, body = %s", callbackResponse.Code, callbackResponse.Header().Get("Location"), callbackResponse.Body.String())
	}
	h.assertNoSecrets(t, callbackResponse.Header().Get("Location")+callbackResponse.Body.String())

	session := feishuE2ESession{
		sessionCookie: findE2ECookie(callbackResponse.Result().Cookies(), SessionCookieName),
		csrfCookie:    findE2ECookie(callbackResponse.Result().Cookies(), CSRFCookieName),
	}
	if session.sessionCookie == nil || session.csrfCookie == nil {
		t.Fatalf("callback cookies = %#v", callbackResponse.Result().Cookies())
	}
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(session.sessionCookie)
	meResponse := httptest.NewRecorder()
	h.router.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK || !strings.Contains(meResponse.Body.String(), `"display_name":"`+identity+`"`) {
		t.Fatalf("authenticated me status/body = %d/%s", meResponse.Code, meResponse.Body.String())
	}
	if identity == "owner" {
		userID := h.store.userIDByOpenID("ou_owner")
		h.store.kbOwners[uuid.MustParse(h.ownerKBID)] = uuid.MustParse(userID)
	}
	return session
}

func (h *feishuE2EHarness) mutate(t *testing.T, session feishuE2ESession, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://app.example.test")
	request.Header.Set(CSRFHeaderName, session.csrfCookie.Value)
	request.AddCookie(session.sessionCookie)
	request.AddCookie(session.csrfCookie)
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, request)
	h.assertNoSecrets(t, response.Header().Get("Location")+response.Body.String())
	return response
}

func (h *feishuE2EHarness) assertLoadedSnapshot(t *testing.T, documentID string) {
	t.Helper()
	snapshot, ok := h.snapshots[documentID]
	if !ok || snapshot.title != "E2E document" || snapshot.markdown != "E2E body\n" || snapshot.remoteRevision != "7" || snapshot.sourceURL != "https://acme.feishu.cn/docx/e2eDoc" {
		t.Fatalf("loaded snapshot = %#v", snapshot)
	}
	metadata := string(snapshot.metadata)
	h.assertNoSecrets(t, metadata)
	var sourceMetadata domain.SourceMetadata
	if err := json.Unmarshal(snapshot.metadata, &sourceMetadata); err != nil {
		t.Fatalf("decode source metadata: %v", err)
	}
	values := sourceMetadata.Values()
	if values.SourceType != domain.ResourceDocx || values.SectionPath != "E2E document" || values.RemoteRevision != "7" || values.SourceLocator.String() != snapshot.sourceURL {
		t.Fatalf("source metadata = %#v", values)
	}
	if h.queueCalls == 0 || h.sourceCalls < 2 {
		t.Fatalf("import did not traverse queue/Feishu source: %d/%d", h.queueCalls, h.sourceCalls)
	}
}

func (h *feishuE2EHarness) assertNextQueuedSync(t *testing.T, documentID, revision string) {
	t.Helper()
	if h.queue == nil || len(h.queue.jobs) == 0 || h.queue.jobs[0] != (feishuE2EQueuedSync{documentID: documentID, requestedRevision: revision}) {
		t.Fatalf("next queued sync = %#v, want document=%s revision=%s", h.queue.jobs, documentID, revision)
	}
}

func (h *feishuE2EHarness) drainNextSync(t *testing.T) {
	t.Helper()
	if err := h.queue.DrainNext(context.Background()); err != nil {
		t.Fatalf("drain recorded sync: %v", err)
	}
}

func (h *feishuE2EHarness) assertNoSecrets(t *testing.T, value string) {
	t.Helper()
	secrets := []string{
		"e2e-app-secret-canary", "e2e-code-owner", "e2e-code-other",
		"e2e-access-owner", "e2e-access-other", "e2e-refresh-owner", "e2e-refresh-other",
		"access_token", "refresh_token", "E2E body",
	}
	secrets = append(secrets, h.oauthStates...)
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			t.Fatalf("value exposed %q: %s", secret, value)
		}
	}
}

func (h *feishuE2EHarness) serveFakeOAuth(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/open-apis/authen/v1/authorize":
		h.oauthAuthorizeCalls++
		identity := h.pendingIdentity
		state := r.URL.Query().Get("state")
		if identity == "" || state == "" || r.URL.Query().Get("app_id") != "cli_e2e" || r.URL.Query().Get("redirect_uri") != "https://backend.example.test/api/v1/auth/feishu/callback" {
			t.Errorf("unexpected authorize request")
			http.Error(w, "invalid authorize request", http.StatusBadRequest)
			return
		}
		code := "e2e-code-" + identity
		h.oauthStates = append(h.oauthStates, state)
		h.authorizedCodes[code] = identity
		destination := "https://backend.example.test/api/v1/auth/feishu/callback?state=" + url.QueryEscape(state) + "&code=" + url.QueryEscape(code)
		http.Redirect(w, r, destination, http.StatusFound)
	case "/open-apis/authen/v2/oauth/token":
		h.oauthTokenCalls++
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid token request", http.StatusBadRequest)
			return
		}
		identity, ok := h.authorizedCodes[request["code"]]
		if !ok || request["client_id"] != "cli_e2e" || request["client_secret"] != "e2e-app-secret-canary" {
			http.Error(w, "invalid token request", http.StatusUnauthorized)
			return
		}
		accessToken := "e2e-access-" + identity
		h.accessTokens[accessToken] = identity
		writeE2EJSON(w, map[string]any{
			"code": 0, "access_token": accessToken, "refresh_token": "e2e-refresh-" + identity,
			"expires_in": 3600, "refresh_token_expires_in": 7200, "scope": strings.Join(feishuE2ERequiredScopes(), " "),
		})
	case "/open-apis/authen/v1/user_info":
		h.oauthUserInfoCalls++
		accessToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		identity, ok := h.accessTokens[accessToken]
		if !ok {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}
		writeE2EJSON(w, map[string]any{"code": 0, "data": map[string]any{
			"open_id": "ou_" + identity, "union_id": "on_" + identity, "tenant_key": "tenant-e2e",
			"name": identity, "email": identity + "@example.test",
		}})
	default:
		http.NotFound(w, r)
	}
}

func (h *feishuE2EHarness) serveFakeFeishuAPI(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer e2e-access-owner" {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
		return
	}
	h.sourceCalls++
	switch {
	case strings.HasSuffix(r.URL.Path, "/documents/e2eDoc"):
		writeE2EJSON(w, map[string]any{"code": 0, "data": map[string]any{"document": map[string]any{
			"document_id": "e2eDoc", "revision_id": 7, "title": "E2E document",
		}}})
	case strings.HasSuffix(r.URL.Path, "/documents/e2eDoc/blocks"):
		writeE2EJSON(w, map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"block_id": "root", "block_type": 2, "text": map[string]any{
				"elements": []any{map[string]any{"text_run": map[string]any{"content": "E2E body"}}},
			}}}, "has_more": false,
		}})
	default:
		http.NotFound(w, r)
	}
}

func feishuE2ERequiredScopes() []string {
	return []string{
		"offline_access",
		"contact:user.base:readonly",
		"contact:user.email:readonly",
		"docx:document:readonly",
		"sheets:spreadsheet:readonly",
		"bitable:app:readonly",
		"wiki:wiki:readonly",
	}
}

func findE2ECookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func writeE2EJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func readFeishuE2EFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func runtimeRequiredFeishuScopes(t *testing.T, source string) []string {
	t.Helper()
	file := parseFeishuE2EGoSource(t, source)
	constants := make(map[string]string)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			values, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if index >= len(values.Values) {
					continue
				}
				literal, ok := values.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("decode scope constant %s: %v", name.Name, err)
				}
				constants[name.Name] = value
			}
		}
	}

	var scopes []string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "requiredFeishuScopes" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			returned, ok := node.(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 {
				return true
			}
			literal, ok := returned.Results[0].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("requiredFeishuScopes must return a composite literal")
			}
			for _, element := range literal.Elts {
				identifier, ok := element.(*ast.Ident)
				if !ok || constants[identifier.Name] == "" {
					t.Fatalf("requiredFeishuScopes contains an unresolved element")
				}
				scopes = append(scopes, constants[identifier.Name])
			}
			return false
		})
	}
	if len(scopes) == 0 {
		t.Fatal("requiredFeishuScopes return value was not found")
	}
	return scopes
}

func runtimeFeishuEnvironment(t *testing.T, source string) map[string]bool {
	t.Helper()
	file := parseFeishuE2EGoSource(t, source)
	readers := map[string]bool{
		"getEnv": true, "mustEnv": true, "positiveDurationEnv": true, "boundedPositiveIntEnv": true,
	}
	environment := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok || !readers[function.Name] {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatalf("decode env name: %v", err)
		}
		if isFeishuRuntimeEnvironment(name) {
			environment[name] = true
		}
		return true
	})
	return environment
}

func isFeishuRuntimeEnvironment(name string) bool {
	return strings.HasPrefix(name, "FEISHU_") || strings.HasPrefix(name, "SESSION_") ||
		name == "OAUTH_ENCRYPTION_KEY" || name == "FRONTEND_ORIGIN" ||
		name == "BOOTSTRAP_OWNER_FEISHU_OPEN_ID" || name == "INGESTION_JOB_TIMEOUT" ||
		name == "RIVER_RESCUE_STUCK_JOBS_AFTER"
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func documentedFeishuEnvFromTemplate(source string) map[string]bool {
	result := make(map[string]bool)
	for _, line := range strings.Split(source, "\n") {
		name, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && isFeishuRuntimeEnvironment(name) {
			result[name] = true
		}
	}
	return result
}

func documentedFeishuEnvFromGuide(t *testing.T, source string) map[string]bool {
	t.Helper()
	start := strings.Index(source, "## 2. 环境变量")
	end := strings.Index(source, "## 3. Cookie")
	if start < 0 || end <= start {
		t.Fatal("deployment guide environment section not found")
	}
	result := make(map[string]bool)
	identifier := regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
	parts := strings.Split(source[start:end], "`")
	for index := 1; index < len(parts); index += 2 {
		name := parts[index]
		if identifier.MatchString(name) && isFeishuRuntimeEnvironment(name) {
			result[name] = true
		}
	}
	return result
}

func assertFeishuE2ESetEqual(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	missing, extra := make([]string, 0), make([]string, 0)
	for value := range want {
		if !got[value] {
			missing = append(missing, value)
		}
	}
	for value := range got {
		if !want[value] {
			extra = append(extra, value)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("%s mismatch: missing=%v extra=%v", label, missing, extra)
	}
}

func envTemplateValue(t *testing.T, source, name string) string {
	t.Helper()
	for _, line := range strings.Split(source, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == name {
			return value
		}
	}
	t.Fatalf("environment template does not define %s", name)
	return ""
}

func applyEnvironmentTemplate(t *testing.T, source string) {
	t.Helper()
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if ok {
			t.Setenv(name, value)
		}
	}
}

func runtimeConstantExpression(t *testing.T, source, name string) string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "runtime_defaults.go", source, 0)
	if err != nil {
		t.Fatalf("parse runtime defaults: %v", err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			values, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, identifier := range values.Names {
				if identifier.Name != name || index >= len(values.Values) {
					continue
				}
				var rendered bytes.Buffer
				if err := format.Node(&rendered, fileSet, values.Values[index]); err != nil {
					t.Fatalf("format runtime constant %s: %v", name, err)
				}
				return rendered.String()
			}
		}
	}
	t.Fatalf("runtime constant %s not found", name)
	return ""
}

func parseFeishuE2EGoSource(t *testing.T, source string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "runtime_contract.go", source, 0)
	if err != nil {
		t.Fatalf("parse runtime contract source: %v", err)
	}
	return file
}

type feishuE2ESnapshot struct {
	title          string
	markdown       string
	remoteRevision string
	sourceURL      string
	metadata       []byte
}

type feishuE2EDocument struct {
	owner uuid.UUID
	row   generated.Document
}

type feishuE2EStore struct {
	users         map[string]domain.User
	userByOpenID  map[string]string
	accounts      map[string]domain.OAuthAccount
	accountByUser map[string]string
	kbOwners      map[uuid.UUID]uuid.UUID
	documents     map[uuid.UUID]feishuE2EDocument
}

func newFeishuE2EStore() *feishuE2EStore {
	return &feishuE2EStore{
		users: make(map[string]domain.User), userByOpenID: make(map[string]string),
		accounts: make(map[string]domain.OAuthAccount), accountByUser: make(map[string]string),
		kbOwners: make(map[uuid.UUID]uuid.UUID), documents: make(map[uuid.UUID]feishuE2EDocument),
	}
}

func (s *feishuE2EStore) UpsertOAuthIdentity(_ context.Context, identity domain.FeishuIdentity, account domain.OAuthAccount) (domain.User, domain.OAuthAccount, error) {
	userID := s.userByOpenID[identity.OpenID]
	if userID == "" {
		userID = uuid.NewString()
		s.userByOpenID[identity.OpenID] = userID
	}
	user := domain.User{ID: userID, DisplayName: identity.DisplayName, Email: identity.Email, AvatarURL: identity.AvatarURL}
	s.users[userID] = user
	accountID := s.accountByUser[userID]
	if accountID == "" {
		accountID = uuid.NewString()
		s.accountByUser[userID] = accountID
	}
	account.ID, account.UserID, account.ProviderUserID = accountID, userID, identity.OpenID
	account.AccessTokenEncrypted = slices.Clone(account.AccessTokenEncrypted)
	account.RefreshTokenEncrypted = slices.Clone(account.RefreshTokenEncrypted)
	account.Scopes = slices.Clone(account.Scopes)
	s.accounts[accountID] = account
	return user, account, nil
}

func (s *feishuE2EStore) OAuthAccount(_ context.Context, id string) (domain.OAuthAccount, error) {
	account, ok := s.accounts[id]
	if !ok {
		return domain.OAuthAccount{}, errors.New("account not found")
	}
	return account, nil
}

func (s *feishuE2EStore) UpdateOAuthTokens(_ context.Context, account domain.OAuthAccount) error {
	if _, ok := s.accounts[account.ID]; !ok {
		return errors.New("account not found")
	}
	s.accounts[account.ID] = account
	return nil
}

func (s *feishuE2EStore) MarkOAuthAccountReauthRequired(_ context.Context, id string) error {
	account, ok := s.accounts[id]
	if !ok {
		return errors.New("account not found")
	}
	account.ReauthRequired = true
	s.accounts[id] = account
	return nil
}

func (s *feishuE2EStore) User(_ context.Context, id string) (domain.User, error) {
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, errors.New("user not found")
	}
	return user, nil
}

func (s *feishuE2EStore) userIDByOpenID(openID string) string { return s.userByOpenID[openID] }

func (s *feishuE2EStore) GetOAuthAccountForUser(_ context.Context, userID uuid.UUID) (generated.OauthAccount, error) {
	accountID := s.accountByUser[userID.String()]
	account, ok := s.accounts[accountID]
	if !ok {
		return generated.OauthAccount{}, pgx.ErrNoRows
	}
	return oauthAccountToGenerated(account), nil
}

func (s *feishuE2EStore) CreateFeishuDocumentAndEnqueue(ctx context.Context, input service.CreateFeishuDocumentInput, enqueuer service.FeishuSyncEnqueuer) (generated.Document, error) {
	account, ok := s.accounts[input.OAuthAccountID.String()]
	if s.kbOwners[input.KBID] != input.OwnerUserID || !ok || account.UserID != input.OwnerUserID.String() {
		return generated.Document{}, pgx.ErrNoRows
	}
	now := time.Now()
	sourceURL := input.SourceURL
	documentID := uuid.New()
	row := generated.Document{
		ID: documentID, KbID: input.KBID, SourceType: input.SourceType, SourceRef: input.SourceRef,
		SourceUrl: &sourceURL, Title: input.Title, Status: string(domain.StatusPending), Metadata: []byte("{}"),
		OauthAccountID: pgtype.UUID{Bytes: input.OAuthAccountID, Valid: true}, SyncStatus: "idle", CreatedAt: now, UpdatedAt: now,
	}
	s.documents[documentID] = feishuE2EDocument{owner: input.OwnerUserID, row: row}
	if err := enqueuer.EnqueueFeishuSync(ctx, documentID.String(), service.InitialFeishuRevision); err != nil {
		delete(s.documents, documentID)
		return generated.Document{}, err
	}
	return row, nil
}

func (s *feishuE2EStore) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	document, ok := s.documents[arg.ID]
	if !ok || !arg.OwnerUserID.Valid || document.owner != arg.OwnerUserID.Bytes {
		return generated.Document{}, pgx.ErrNoRows
	}
	return document.row, nil
}

func (s *feishuE2EStore) GetFeishuDocumentForOwnerAndAccount(_ context.Context, arg generated.GetFeishuDocumentForOwnerAndAccountParams) (generated.Document, error) {
	document, ok := s.documents[arg.ID]
	if !ok || !arg.OwnerUserID.Valid || document.owner != arg.OwnerUserID.Bytes || !document.row.OauthAccountID.Valid || document.row.OauthAccountID.Bytes != arg.OauthAccountID {
		return generated.Document{}, pgx.ErrNoRows
	}
	return document.row, nil
}

func oauthAccountToGenerated(account domain.OAuthAccount) generated.OauthAccount {
	row := generated.OauthAccount{
		ID: uuid.MustParse(account.ID), UserID: uuid.MustParse(account.UserID), Provider: account.Provider,
		ProviderUserID: account.ProviderUserID, TenantKey: account.TenantKey,
		AccessTokenEncrypted: slices.Clone(account.AccessTokenEncrypted), RefreshTokenEncrypted: slices.Clone(account.RefreshTokenEncrypted),
		AccessTokenExpiresAt: account.AccessTokenExpiresAt, Scopes: slices.Clone(account.Scopes), ReauthRequired: account.ReauthRequired,
	}
	if account.RefreshTokenExpiresAt != nil {
		row.RefreshTokenExpiresAt = pgtype.Timestamptz{Time: *account.RefreshTokenExpiresAt, Valid: true}
	}
	return row
}

type feishuE2EQueue struct {
	harness  *feishuE2EHarness
	store    *feishuE2EStore
	auth     *service.Auth
	resolver *feishu.URLResolver
	loader   *feishu.DocxLoader
	jobs     []feishuE2EQueuedSync
}

type feishuE2EQueuedSync struct {
	documentID        string
	requestedRevision string
}

func (q *feishuE2EQueue) EnqueueFeishuSync(_ context.Context, documentID, requestedRevision string) error {
	q.harness.queueCalls++
	if _, err := uuid.Parse(documentID); err != nil {
		return err
	}
	q.jobs = append(q.jobs, feishuE2EQueuedSync{documentID: documentID, requestedRevision: requestedRevision})
	return nil
}

// DrainNext is the in-memory worker boundary: HTTP only records jobs; this
// explicit driver performs the real auth/resolver/loader source call.
func (q *feishuE2EQueue) DrainNext(ctx context.Context) error {
	if len(q.jobs) == 0 {
		return errors.New("no recorded Feishu sync job")
	}
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	id, err := uuid.Parse(job.documentID)
	if err != nil {
		return err
	}
	document, ok := q.store.documents[id]
	if !ok || document.row.SourceUrl == nil || !document.row.OauthAccountID.Valid {
		return fmt.Errorf("document not available for sync")
	}
	accountID := uuid.UUID(document.row.OauthAccountID.Bytes).String()
	accessToken, err := q.auth.AccessToken(ctx, accountID)
	if err != nil {
		return err
	}
	ref, err := q.resolver.Resolve(*document.row.SourceUrl)
	if err != nil {
		return err
	}
	canonical, err := q.loader.Load(ctx, ref, accessToken)
	if err != nil {
		return err
	}
	metadata, err := canonical.SourceMetadata.MarshalJSON()
	if err != nil {
		return err
	}
	q.harness.snapshots[job.documentID] = feishuE2ESnapshot{
		title: canonical.Title, markdown: canonical.Markdown, remoteRevision: canonical.RemoteRevision,
		sourceURL: canonical.SafeSourceURL.String(), metadata: metadata,
	}
	document.row.RemoteRevision = &canonical.RemoteRevision
	q.store.documents[id] = document
	return nil
}
