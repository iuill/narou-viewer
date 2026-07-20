package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionjobs"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionruntime"
	"narou-viewer/apps/viewer-api-go/internal/application/fetchercommands"
	"narou-viewer/apps/viewer-api-go/internal/application/libraryview"
	"narou-viewer/apps/viewer-api-go/internal/application/readerassistant"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/application/readerview"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

const maxJSONBodyBytes int64 = 1 << 20

const nonStreamingLLMTimeout = 55 * time.Second
const streamingWriteDeadline = 180 * time.Second

const apiContractVersion = "1"

const apiContractVersionHeader = "X-Narou-Viewer-Api-Contract-Version"
const apiContractMinVersionHeader = "X-Narou-Viewer-Min-Api-Contract-Version"
const apiClientBuildHeader = "X-Narou-Viewer-Client-Build"
const apiReloadRequiredHeader = "X-Narou-Viewer-Reload-Required"
const apiRequestIDHeader = "X-Request-Id"

type apiErrorResponse struct {
	Error     string         `json:"error"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

type Server struct {
	mux                *http.ServeMux
	ctx                context.Context
	cancel             context.CancelFunc
	preferredMode      string
	dataDir            string
	library            *library.Service
	publications       *publications.Service
	stateStore         *store.Store
	fetcherClient      *fetcher.Client
	fetcherCommands    *fetchercommands.Service
	libraryView        *libraryview.Service
	readerAssistant    *readerassistant.Service
	readerView         *readerview.Service
	extraction         *extractionruntime.Runtime
	extractionJobQueue *extractionjobs.Service
	stateInitErr       error
	extractionJobs     *appextraction.JobCoordinator
	storageProgress    *storageUsageProgressStore
	backgroundOnce     sync.Once
}

type ServerDependencies struct {
	DataDir                         string
	Library                         *library.Service
	Publications                    *publications.Service
	StateStore                      *store.Store
	FetcherClient                   *fetcher.Client
	FetcherCommand                  *fetchercommands.Service
	LibraryView                     *libraryview.Service
	ReaderAssistant                 *readerassistant.Service
	ReaderView                      *readerview.Service
	Extraction                      *extractionruntime.Runtime
	ExtractionQueue                 *extractionjobs.Service
	ExtractionJobCoordinator        *appextraction.JobCoordinator
	ExtractionJobCoordinatorFactory func(extractionruntime.Workflow, string, extractionruntime.Logger) *appextraction.JobCoordinator
	StateInitErr                    error
}

func NewServerWithDependencies(deps ServerDependencies) http.Handler {
	serverCtx, cancel := context.WithCancel(context.Background())
	publicationService := deps.Publications
	if publicationService == nil {
		publicationService = publications.NewService(filepath.Join(deps.DataDir, "state"))
	}
	libraryViewService := deps.LibraryView
	if libraryViewService == nil {
		libraryViewService = libraryview.NewService(deps.Library, deps.StateStore, publicationService)
	}
	stateDir := filepath.Join(deps.DataDir, "state")
	textCache := readertextcache.New(stateDir)
	readerViewService := deps.ReaderView
	if readerViewService == nil {
		readerViewService = readerview.NewServiceWithTextCache(deps.Library, deps.StateStore, textCache)
	}
	readerAssistantService := deps.ReaderAssistant
	if readerAssistantService == nil {
		readerAssistantService = readerassistant.NewService(readerassistant.Dependencies{
			Library:     deps.Library,
			Settings:    deps.StateStore,
			StateDir:    stateDir,
			UsageDBPath: filepath.Join(deps.DataDir, "state", "ai_usage.sqlite"),
			TextCache:   textCache,
		})
	}
	extractionRuntime := deps.Extraction
	if extractionRuntime == nil {
		extractionRuntime = extractionruntime.NewRuntime(extractionruntime.RuntimeDependencies{
			StateDir:    stateDir,
			UsageDBPath: filepath.Join(stateDir, "ai_usage.sqlite"),
			Library:     deps.Library,
			Settings:    deps.StateStore,
			Logger:      logExtractionTiming,
		})
	}
	extractionJobQueue := deps.ExtractionQueue
	if extractionJobQueue == nil {
		extractionJobQueue = extractionjobs.NewService(filepath.Join(deps.DataDir, "state"), deps.Library, deps.StateStore)
	}
	s := &Server{
		mux:                http.NewServeMux(),
		ctx:                serverCtx,
		cancel:             cancel,
		preferredMode:      "heuristic",
		dataDir:            deps.DataDir,
		library:            deps.Library,
		publications:       publicationService,
		stateStore:         deps.StateStore,
		fetcherClient:      deps.FetcherClient,
		fetcherCommands:    deps.FetcherCommand,
		libraryView:        libraryViewService,
		readerAssistant:    readerAssistantService,
		readerView:         readerViewService,
		extraction:         extractionRuntime,
		extractionJobQueue: extractionJobQueue,
		stateInitErr:       deps.StateInitErr,
		storageProgress:    newStorageUsageProgressStore(),
	}
	s.extractionJobs = deps.ExtractionJobCoordinator
	if s.extractionJobs == nil && deps.ExtractionJobCoordinatorFactory != nil {
		s.extractionJobs = deps.ExtractionJobCoordinatorFactory(s.extraction.Workflow(), s.stateDir(), logExtractionTiming)
	}
	if s.extractionJobs == nil {
		s.extractionJobs = appextraction.NewJobCoordinator(s.stateDir(), s.extraction.ProcessJob)
	}
	s.routes()
	return s
}

func (s *Server) StartBackground(ctx context.Context) {
	if s == nil {
		return
	}
	s.backgroundOnce.Do(func() {
		if ctx == nil {
			ctx = s.ctx
		}
		s.extractionJobs.Recover()
		s.extractionJobs.Kick(ctx)
	})
}

func (s *Server) Shutdown(context.Context) error {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *Server) extractionRuntime() *extractionruntime.Runtime {
	if s == nil {
		return extractionruntime.NewRuntime(extractionruntime.RuntimeDependencies{Logger: logExtractionTiming})
	}
	if s.extraction == nil {
		stateDir := s.stateDir()
		s.extraction = extractionruntime.NewRuntime(extractionruntime.RuntimeDependencies{
			StateDir:    stateDir,
			UsageDBPath: filepath.Join(stateDir, "ai_usage.sqlite"),
			Library:     s.library,
			Settings:    s.stateStore,
			Logger:      logExtractionTiming,
		})
	}
	return s.extraction
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isAPIRequest(r) {
		ensureAPIRequestID(w, r)
	}
	if handleCORS(w, r) {
		return
	}
	if requiresCurrentAPIContract(r) && !hasCurrentAPIContract(r) {
		writeClientUpdateRequired(w)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func extendStreamingWriteDeadline(w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(streamingWriteDeadline)); err != nil {
		fmt.Printf("viewer-api-go: failed to extend streaming write deadline: %v\n", err)
	}
}

func nonStreamingLLMContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, nonStreamingLLMTimeout)
}

func handleCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	if !isAllowedCORSOrigin(r, origin) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		if isUnsafeMethod(r.Method) {
			writeError(w, http.StatusForbidden, "Origin is not allowed.")
			return true
		}
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", strings.Join([]string{
		"Content-Type",
		"Authorization",
		"If-None-Match",
		apiContractVersionHeader,
		apiClientBuildHeader,
		apiRequestIDHeader,
	}, ", "))
	w.Header().Set("Access-Control-Expose-Headers", strings.Join([]string{"ETag", apiContractVersionHeader, apiContractMinVersionHeader, apiReloadRequiredHeader, apiRequestIDHeader}, ", "))
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func isUnsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func requiresCurrentAPIContract(r *http.Request) bool {
	if r == nil || !isUnsafeMethod(r.Method) {
		return false
	}
	return isAPIRequest(r)
}

func hasCurrentAPIContract(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get(apiContractVersionHeader)) == apiContractVersion
}

func isAPIRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	return r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")
}

func ensureAPIRequestID(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(w.Header().Get(apiRequestIDHeader)) != "" {
		return
	}
	w.Header().Set(apiRequestIDHeader, resolveAPIRequestID(r))
}

func resolveAPIRequestID(r *http.Request) string {
	if r != nil {
		requestID := strings.TrimSpace(r.Header.Get(apiRequestIDHeader))
		if isValidAPIRequestID(requestID) {
			return requestID
		}
	}
	return generateAPIRequestID()
}

func isValidAPIRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func generateAPIRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func isAllowedCORSOrigin(r *http.Request, origin string) bool {
	if configured := strings.TrimSpace(os.Getenv("VIEWER_API_ALLOWED_ORIGINS")); configured != "" {
		for _, item := range strings.Split(configured, ",") {
			if strings.TrimSpace(item) == origin {
				return true
			}
		}
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if r != nil && originMatchesRequestHost(parsed, r) {
		return true
	}
	return isDevelopmentCORSFallbackEnabled() && isDevelopmentCORSHost(host)
}

func isDevelopmentCORSFallbackEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("VIEWER_API_DEV_CORS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isDevelopmentCORSHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func originMatchesRequestHost(origin *url.URL, r *http.Request) bool {
	originHost := normalizeOriginHost(origin)
	if originHost == "" {
		return false
	}
	for _, candidate := range []string{
		r.Host,
		r.Header.Get("X-Forwarded-Host"),
	} {
		if normalizeHostPort(candidate, origin.Scheme) == originHost {
			return true
		}
	}
	return false
}

func normalizeOriginHost(origin *url.URL) string {
	host := strings.TrimSpace(origin.Host)
	if host == "" {
		return ""
	}
	return normalizeHostPort(host, origin.Scheme)
}

func normalizeHostPort(host string, scheme string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.Contains(host, ",") {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	parsedHost := host
	port := ""
	if strings.Contains(host, ":") {
		if value, parsedPort, err := net.SplitHostPort(host); err == nil {
			parsedHost = value
			port = parsedPort
		}
	}
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return strings.ToLower(strings.Trim(parsedHost, "[]")) + ":" + port
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set(apiContractVersionHeader, apiContractVersion)
	w.Header().Set(apiContractMinVersionHeader, apiContractVersion)
	requestID := strings.TrimSpace(w.Header().Get(apiRequestIDHeader))
	if requestID == "" {
		requestID = generateAPIRequestID()
		w.Header().Set(apiRequestIDHeader, requestID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(normalizeAPIErrorPayload(status, payload, requestID))
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeAPIError(w, status, defaultAPIErrorCode(status), message, nil)
}

func normalizeAPIErrorPayload(status int, payload any, requestID string) any {
	if status < 400 {
		return payload
	}
	switch typed := payload.(type) {
	case apiErrorResponse:
		if typed.RequestID == "" {
			typed.RequestID = requestID
		}
		return typed
	case map[string]string:
		message := strings.TrimSpace(typed["error"])
		if message == "" {
			return payload
		}
		code := typed["code"]
		if code == "" {
			code = defaultAPIErrorCode(status)
		}
		return apiErrorResponse{
			Error:     message,
			Code:      normalizeAPIErrorCode(code),
			Message:   message,
			RequestID: requestID,
		}
	case map[string]any:
		rawMessage, _ := typed["error"].(string)
		message := strings.TrimSpace(rawMessage)
		if message == "" {
			return payload
		}
		next := map[string]any{}
		for key, value := range typed {
			next[key] = value
		}
		code, _ := typed["code"].(string)
		if code == "" {
			code = defaultAPIErrorCode(status)
		}
		next["code"] = normalizeAPIErrorCode(code)
		if _, ok := next["message"]; !ok {
			next["message"] = message
		}
		if _, ok := next["requestId"]; !ok && requestID != "" {
			next["requestId"] = requestID
		}
		if _, ok := next["details"]; !ok {
			details := map[string]any{}
			for key, value := range typed {
				if key == "error" || key == "code" || key == "message" || key == "details" || key == "requestId" {
					continue
				}
				details[key] = value
			}
			if len(details) > 0 {
				next["details"] = details
			}
		}
		return next
	default:
		return payload
	}
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string, details map[string]any) {
	writeJSON(w, status, apiErrorResponse{
		Error:     message,
		Code:      normalizeAPIErrorCode(code),
		Message:   message,
		Details:   details,
		RequestID: strings.TrimSpace(w.Header().Get(apiRequestIDHeader)),
	})
}

func writeAPIErrorWithFields(w http.ResponseWriter, status int, code string, message string, details map[string]any, fields map[string]any) {
	payload := map[string]any{
		"error":   message,
		"code":    normalizeAPIErrorCode(code),
		"message": message,
	}
	if details != nil {
		payload["details"] = details
	}
	if requestID := strings.TrimSpace(w.Header().Get(apiRequestIDHeader)); requestID != "" {
		payload["requestId"] = requestID
	}
	for key, value := range fields {
		if key == "error" || key == "code" || key == "message" || key == "details" || key == "requestId" {
			continue
		}
		payload[key] = value
	}
	writeJSON(w, status, payload)
}

func writeClientUpdateRequired(w http.ResponseWriter) {
	w.Header().Set(apiReloadRequiredHeader, "1")
	writeAPIError(w, http.StatusUpgradeRequired, "CLIENT_UPDATE_REQUIRED", "Client update required.", map[string]any{
		"minApiContractVersion": apiContractVersion,
	})
}

func normalizeAPIErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "ERROR"
	}
	return code
}

func defaultAPIErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusGone:
		return "GONE"
	case http.StatusUnsupportedMediaType:
		return "UNSUPPORTED_MEDIA_TYPE"
	case http.StatusServiceUnavailable:
		return "SERVICE_UNAVAILABLE"
	case http.StatusBadGateway:
		return "BAD_GATEWAY"
	default:
		if status >= 500 {
			return "INTERNAL_SERVER_ERROR"
		}
		return "ERROR"
	}
}

func decodeObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	defer r.Body.Close()
	limitedBody := http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(limitedBody)
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		if err == io.EOF {
			return map[string]any{}, true
		}
		return map[string]any{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return map[string]any{}, false
	}
	if decoded == nil {
		return map[string]any{}, true
	}
	return decoded, true
}

func decodeObjectOrBadRequest(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if !hasJSONContentType(r) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json.")
		return nil, false
	}
	body, ok := decodeObject(w, r)
	if !ok {
		writeError(w, http.StatusBadRequest, "Malformed JSON body.")
		return nil, false
	}
	return body, true
}

func decodeJSONOrBadRequest[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var zero T
	if !hasJSONContentType(r) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json.")
		return zero, false
	}
	defer r.Body.Close()
	limitedBody := http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(limitedBody)
	var decoded T
	if err := decoder.Decode(&decoded); err != nil {
		if err == io.EOF {
			return decoded, true
		}
		writeError(w, http.StatusBadRequest, "Malformed JSON body.")
		return zero, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Malformed JSON body.")
		return zero, false
	}
	return decoded, true
}

func hasJSONContentType(r *http.Request) bool {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

func methodOnly(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	if len(methods) > 0 {
		w.Header().Set("allow", strings.Join(methods, ", "))
	}
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed.")
	return false
}

func isPositiveIntegerString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, isDigits(v) && v != "0"
	case float64:
		if v <= 0 || v != float64(int64(v)) {
			return "", false
		}
		return fmt.Sprintf("%.0f", v), true
	default:
		return "", false
	}
}

func isNonNegativeIntegerString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, isDigits(v)
	case float64:
		if v < 0 || v != float64(int64(v)) {
			return "", false
		}
		return fmt.Sprintf("%.0f", v), true
	default:
		return "", false
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func trimPathValue(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		decoded = value
	}
	return strings.TrimSpace(decoded)
}
