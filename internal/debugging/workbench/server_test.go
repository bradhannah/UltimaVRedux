package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

type controlPlaneStub struct {
	requests   chan debugging.Request
	results    chan debugging.Result
	submitErr  error
	onSubmit   func(debugging.Request)
	generation atomic.Uint64
}

func newControlPlaneStub() *controlPlaneStub {
	stub := &controlPlaneStub{
		requests:   make(chan debugging.Request, 16),
		results:    make(chan debugging.Result, 16),
		submitErr:  nil,
		onSubmit:   nil,
		generation: atomic.Uint64{},
	}
	stub.generation.Store(7)

	return stub
}

func (s *controlPlaneStub) SubmitDebugRequest(request debugging.Request) error {
	if s.submitErr != nil {
		return s.submitErr
	}
	s.requests <- request
	if s.onSubmit != nil {
		s.onSubmit(request)
	}

	return nil
}

func (s *controlPlaneStub) NextDebugResult() (debugging.Result, bool) {
	select {
	case result := <-s.results:
		return result, true
	default:
		var empty debugging.Result

		return empty, false
	}
}

func (s *controlPlaneStub) DebugDiscoverySnapshot() debugging.DiscoverySnapshot {
	return debugging.DiscoverySnapshot{
		SceneGeneration: s.generation.Load(),
		OptionsRevision: 3,
		Commands:        debugging.NewInitialCatalog().Definitions(),
		OptionSets: []debugging.OptionSet{{
			Provider:          debugging.OptionProviderActivePartyCharacters,
			Choices:           []debugging.Choice{{Value: "0.QXZhdGFy", Label: "Position 1 · Avatar", Metadata: nil}},
			UnavailableReason: "",
		}, {
			Provider:          debugging.OptionProviderActivePartyPositions,
			Choices:           []debugging.Choice{{Value: "1", Label: "Position 1 · Avatar", Metadata: nil}},
			UnavailableReason: "",
		}},
	}
}

func (s *controlPlaneStub) DebugSceneGeneration() uint64 {
	return s.generation.Load()
}

func TestNewServerRejectsNonLoopbackAddress(t *testing.T) {
	config := DefaultConfig()
	config.Address = "0.0.0.0:8585"

	_, err := NewServer(newControlPlaneStub(), config)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestServerServesInjectedWorkbenchBootstrap(t *testing.T) {
	server, _ := startTestServer(t, newControlPlaneStub())

	response, err := http.Get(server.URL())
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, string(body), server.token)
	assert.NotContains(t, string(body), "__U5R_WORKBENCH_TOKEN__")
	assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
}

func TestServerServesAuthorizedTileAtlasOnly(t *testing.T) {
	server, _ := startTestServer(t, newControlPlaneStub())

	response, err := http.Get(server.URL() + "/api/tile-atlas.png")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)

	response, err = http.Get(server.URL() + "/api/tile-atlas.png?token=" + url.QueryEscape(server.token))
	require.NoError(t, err)
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "image/png", response.Header.Get("Content-Type"))
	assert.NotEmpty(t, content)
}

func TestServerServesIndependentReadOnlyLogWindow(t *testing.T) {
	server, _ := startTestServer(t, newControlPlaneStub())
	response, err := http.Get(server.URL() + "/logs")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Contains(t, string(body), server.logToken)
	assert.NotContains(t, string(body), server.token)
	assert.Nil(t, server.active)

	logger := logging.GetGlobalLogger()
	beforeEnabled, beforeCategories := logger.IsEnabled(), logger.GetCategoryStates()
	t.Cleanup(func() { logger.SetEnabled(beforeEnabled); logger.SetCategoryStates(beforeCategories) })
	logger.SetEnabled(true)
	logger.SetCategoryEnabled(logging.CategorySystem, true)
	logger.Infof(logging.CategorySystem, "browser log test")

	request, err := http.NewRequest(http.MethodGet, server.URL()+"/api/logs?after=0&limit=10", nil)
	require.NoError(t, err)
	request.Header.Set(logAuthHeader, server.logToken)
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	var batch logging.EntryBatch
	require.NoError(t, json.NewDecoder(response.Body).Decode(&batch))
	require.NotEmpty(t, batch.Entries)
	assert.Equal(t, "browser log test", batch.Entries[len(batch.Entries)-1].Message)
	assert.Nil(t, server.active)
}

func TestServerRejectsUnauthorizedAndInvalidLogQueries(t *testing.T) {
	server, _ := startTestServer(t, newControlPlaneStub())
	response, err := http.Get(server.URL() + "/api/logs")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)

	request, err := http.NewRequest(http.MethodGet, server.URL()+"/api/logs?limit=999", nil)
	require.NoError(t, err)
	request.Header.Set(logAuthHeader, server.logToken)
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestServerDiscoversCatalogAndCompletesCommand(t *testing.T) {
	control := newControlPlaneStub()
	server, _ := startTestServer(t, control)
	connection := dialWorkbench(t, server)
	defer connection.CloseNow()

	sessionID := startSession(t, connection, "")
	writeEnvelope(t, connection, newEnvelope("catalog-1", messageCatalogRequest, struct{}{}))
	catalogMessage := readEnvelope(t, connection)
	assert.Equal(t, messageCatalogSnapshot, catalogMessage.Type)
	var catalog catalogSnapshotPayload
	require.NoError(t, json.Unmarshal(catalogMessage.Payload, &catalog))
	assert.Equal(t, uint64(7), catalog.SceneGeneration)
	assert.Equal(t, uint64(3), catalog.OptionsRevision)
	assert.Len(t, catalog.Commands, len(control.DebugDiscoverySnapshot().Commands))
	require.Len(t, catalog.OptionSets, 2)
	expectedSnapshot, err := json.Marshal(control.DebugDiscoverySnapshot())
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedSnapshot), string(catalogMessage.Payload),
		"WebSocket discovery must preserve the complete command inventory, constraints, and options")

	writeEnvelope(t, connection, newEnvelope("request-1", messageCommandExecute, commandExecutePayload{
		CommandID:               debugging.CommandLoggingStatus,
		ExpectedSceneGeneration: 7,
		ExpectedOptionsRevision: 3,
		Arguments:               nil,
	}))
	accepted := readEnvelope(t, connection)
	assert.Equal(t, messageCommandAccepted, accepted.Type)

	request := <-control.requests
	assert.Equal(t, sessionID, request.SessionID)
	assert.Equal(t, "request-1", request.ID)
	assert.Equal(t, uint64(3), request.ExpectedOptionsRevision)
	result := debugging.SuccessResult(request, debugging.ExecutionQuery, "loaded", json.RawMessage(`{"enabled":true}`))
	result.Invocation = debugging.NewNormalizedInvocation(debugging.Request{
		CommandID: request.CommandID,
		Arguments: map[string]json.RawMessage{
			"count":   json.RawMessage(`2`),
			"enabled": json.RawMessage(`true`),
			"mode":    json.RawMessage(`"all"`),
		},
	})
	result.SceneGeneration = 7
	control.results <- result

	completed := readEnvelope(t, connection)
	assert.Equal(t, messageCommandCompleted, completed.Type)
	var completedResult debugging.Result
	require.NoError(t, json.Unmarshal(completed.Payload, &completedResult))
	assert.True(t, completedResult.Success)
	assert.Equal(t, sessionID, completedResult.SessionID)
	require.NotNil(t, completedResult.Invocation)
	assert.Equal(t, debugging.CommandLoggingStatus, completedResult.Invocation.CommandID)
	assert.JSONEq(t, `2`, string(completedResult.Invocation.Arguments["count"]))
	assert.JSONEq(t, `true`, string(completedResult.Invocation.Arguments["enabled"]))
	assert.JSONEq(t, `"all"`, string(completedResult.Invocation.Arguments["mode"]))
	invocationJSON, err := json.Marshal(completedResult.Invocation)
	require.NoError(t, err)
	var invocationObject map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(invocationJSON, &invocationObject))
	assert.Len(t, invocationObject, 2)
	assert.Contains(t, invocationObject, "commandId")
	assert.Contains(t, invocationObject, "arguments")
}

func TestServerReplaysUnacknowledgedResultAfterReconnect(t *testing.T) {
	control := newControlPlaneStub()
	server, _ := startTestServer(t, control)
	connection := dialWorkbench(t, server)
	sessionID := startSession(t, connection, "")

	writeEnvelope(t, connection, newEnvelope("request-1", messageCommandExecute, commandExecutePayload{
		CommandID: debugging.CommandLoggingStatus,
	}))
	assert.Equal(t, messageCommandAccepted, readEnvelope(t, connection).Type)
	request := <-control.requests
	control.results <- debugging.SuccessResult(request, debugging.ExecutionQuery, "loaded", json.RawMessage(`{}`))
	assert.Equal(t, messageCommandCompleted, readEnvelope(t, connection).Type)
	connection.CloseNow()
	waitForControllerRelease(t, server)

	reconnected := dialWorkbench(t, server)
	defer reconnected.CloseNow()
	resumedSessionID := startSession(t, reconnected, sessionID)
	assert.Equal(t, sessionID, resumedSessionID)
	replayed := readEnvelope(t, reconnected)
	assert.Equal(t, messageCommandCompleted, replayed.Type)
	assert.Equal(t, "request-1", replayed.ID)

	writeEnvelope(t, reconnected, newEnvelope("ack-1", messageCommandAck, commandAckPayload{RequestID: "request-1"}))
	require.Eventually(t, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()

		return len(server.sessions[sessionID].operations) == 0
	}, time.Second, time.Millisecond)
}

func TestServerRejectsInvalidAuthorizationAndSecondController(t *testing.T) {
	server, _ := startTestServer(t, newControlPlaneStub())
	websocketURL := strings.Replace(server.URL(), "http://", "ws://", 1) + "/ws?token=invalid"
	_, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{server.URL()}},
	})
	require.Error(t, err)
	require.NotNil(t, response)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)

	crossOriginURL := workbenchWebSocketURL(server)
	_, response, err = websocket.Dial(context.Background(), crossOriginURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://attacker.invalid"}},
	})
	require.Error(t, err)
	require.NotNil(t, response)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)

	connection := dialWorkbench(t, server)
	defer connection.CloseNow()
	secondURL := workbenchWebSocketURL(server)
	_, response, err = websocket.Dial(context.Background(), secondURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{server.URL()}},
	})
	require.Error(t, err)
	require.NotNil(t, response)
	assert.Equal(t, http.StatusConflict, response.StatusCode)
}

func TestClientOutboundSaturationCancelsConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &clientConnection{
		server:    nil,
		conn:      nil,
		cancel:    cancel,
		outbound:  make(chan envelope, 1),
		sessionID: "test-session",
	}
	require.NoError(t, client.enqueue(newEnvelope("one", messageProtocolError, protocolErrorPayload{})))

	err := client.enqueue(newEnvelope("two", messageProtocolError, protocolErrorPayload{}))

	assert.ErrorIs(t, err, ErrOutboundQueueFull)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestServerReportsBridgeSubmissionFailure(t *testing.T) {
	control := newControlPlaneStub()
	control.submitErr = debugging.ErrRequestQueueFull
	server, _ := startTestServer(t, control)
	connection := dialWorkbench(t, server)
	defer connection.CloseNow()
	startSession(t, connection, "")

	writeEnvelope(t, connection, newEnvelope("request-1", messageCommandExecute, commandExecutePayload{
		CommandID: debugging.CommandLoggingStatus,
	}))
	failed := readEnvelope(t, connection)

	assert.Equal(t, messageCommandFailed, failed.Type)
	var result debugging.Result
	require.NoError(t, json.Unmarshal(failed.Payload, &result))
	assert.Equal(t, "request_queue_full", result.ErrorCode)
}

func TestServerSendsAcceptanceBeforeImmediateCompletion(t *testing.T) {
	control := newControlPlaneStub()
	control.onSubmit = func(request debugging.Request) {
		control.results <- debugging.SuccessResult(request, debugging.ExecutionQuery, "loaded", json.RawMessage(`{}`))
	}
	server, _ := startTestServer(t, control)
	connection := dialWorkbench(t, server)
	defer connection.CloseNow()
	startSession(t, connection, "")

	writeEnvelope(t, connection, newEnvelope("request-1", messageCommandExecute, commandExecutePayload{
		CommandID: debugging.CommandLoggingStatus,
	}))

	assert.Equal(t, messageCommandAccepted, readEnvelope(t, connection).Type)
	assert.Equal(t, messageCommandCompleted, readEnvelope(t, connection).Type)
}

func TestServerEvictsAcknowledgedIdleSessionAtCapacity(t *testing.T) {
	server, err := NewServer(newControlPlaneStub(), DefaultConfig())
	require.NoError(t, err)
	server.mu.Lock()
	for index := range maxSessions {
		sessionID := strconv.Itoa(index)
		server.sessions[sessionID] = &sessionState{
			id:         sessionID,
			operations: make(map[string]*operationRecord),
			lastUsed:   time.Unix(int64(index), 0),
		}
	}

	evicted := server.evictIdleSessionLocked()
	server.mu.Unlock()

	assert.True(t, evicted)
	assert.Len(t, server.sessions, maxSessions-1)
	_, oldestExists := server.sessions["0"]
	assert.False(t, oldestExists)
}

func TestServerRejectsControllerAfterShutdownBegins(t *testing.T) {
	server, err := NewServer(newControlPlaneStub(), DefaultConfig())
	require.NoError(t, err)
	server.beginShutdown()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &clientConnection{
		server:    server,
		conn:      nil,
		cancel:    cancel,
		outbound:  make(chan envelope, 1),
		sessionID: "",
	}

	err = server.acquireController(client)

	assert.ErrorIs(t, err, ErrServerStopping)
}

func TestServerBackgroundWaitHonorsContext(t *testing.T) {
	server, err := NewServer(newControlPlaneStub(), DefaultConfig())
	require.NoError(t, err)
	server.clientWG.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err = server.waitForBackgroundWork(ctx)
	server.clientWG.Done()

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func startTestServer(t *testing.T, control *controlPlaneStub) (*Server, Config) {
	t.Helper()
	config := DefaultConfig()
	config.Address = "127.0.0.1:0"
	config.ResultPoll = time.Millisecond
	server, err := NewServer(control, config)
	require.NoError(t, err)
	require.NoError(t, server.Start())
	server.httpServer.SetKeepAlivesEnabled(false)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
	})

	return server, config
}

func dialWorkbench(t *testing.T, server *Server) *websocket.Conn {
	t.Helper()
	connection, response, err := websocket.Dial(context.Background(), workbenchWebSocketURL(server), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{server.URL()}},
	})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	// Catalog snapshots are server output and may be larger than the stricter inbound command limit.
	connection.SetReadLimit(2 * server.config.MaxMessageBytes)

	return connection
}

func workbenchWebSocketURL(server *Server) string {
	return strings.Replace(server.URL(), "http://", "ws://", 1) + "/ws?token=" + url.QueryEscape(server.token)
}

func startSession(t *testing.T, connection *websocket.Conn, requestedSessionID string) string {
	t.Helper()
	writeEnvelope(t, connection, newEnvelope("hello-1", messageSessionHello, sessionHelloPayload{
		SessionID: requestedSessionID,
	}))
	ready := readEnvelope(t, connection)
	require.Equal(t, messageSessionReady, ready.Type)
	var payload sessionReadyPayload
	require.NoError(t, json.Unmarshal(ready.Payload, &payload))
	require.NotEmpty(t, payload.SessionID)

	return payload.SessionID
}

func writeEnvelope(t *testing.T, connection *websocket.Conn, message envelope) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, wsjson.Write(ctx, connection, message))
}

func readEnvelope(t *testing.T, connection *websocket.Conn) envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var message envelope
	require.NoError(t, wsjson.Read(ctx, connection, &message))

	return message
}

func waitForControllerRelease(t *testing.T, server *Server) {
	t.Helper()
	require.Eventually(t, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()

		return server.active == nil
	}, time.Second, time.Millisecond)
}

func TestServerShutdownBeforeStart(t *testing.T) {
	server, err := NewServer(newControlPlaneStub(), DefaultConfig())
	require.NoError(t, err)

	err = server.Shutdown(context.Background())

	assert.True(t, errors.Is(err, ErrNotStarted))
}
