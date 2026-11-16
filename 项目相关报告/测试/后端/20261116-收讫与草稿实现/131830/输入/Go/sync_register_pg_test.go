package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Authentication is an explicit in-process transport fixture. No HTTP server,
// NATS connection or real account authorization is claimed by these PG tests.
type syncRegisterAuthFixture struct{ user User }

func (f syncRegisterAuthFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/internal/authorize" {
		return nil, errors.New("unexpected network-bound dependency in PG-only registration test")
	}
	body, _ := json.Marshal(f.user)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
}

func newSyncRegisterPGApp(t *testing.T) (context.Context, *App, User) {
	t.Helper()
	ctx, db, scope := newSyncSchemaTestRealm(t)
	syncSchemaTestMigration(t, ctx, db, scope)
	configuration := db.Config().Copy()
	guardS2PGConnection(t, configuration, "jobsim_sync_schemas_")
	poolConfiguration, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal("local pool defaults unavailable; details redacted")
	}
	poolConfiguration.ConnConfig, poolConfiguration.MaxConns = configuration, 4
	pool, err := pgxpool.NewWithConfig(ctx, poolConfiguration)
	if err != nil {
		t.Fatal("owned registration PG pool unavailable; details redacted")
	}
	t.Cleanup(pool.Close)
	u := User{ID: newID(), DeviceID: newID(), RestoreEpoch: "1"}
	app := &App{Config: Config{DeploymentID: scope, NATSURL: "nats://127.0.0.1:51222", RoleURLs: map[string]string{"identity": "http://127.0.0.1:1"}}, Role: "sync", DB: pool, Client: &http.Client{Transport: syncRegisterAuthFixture{u}}}
	return ctx, app, u
}

func callSyncRegisterPG(app *App, schema, store, requestID string) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{"schemaVersion": schema, "storeGeneration": store, "requestId": requestID})
	request := httptest.NewRequest("PUT", "/api/sync/device", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	if err := app.syncRegister(recorder, request); err != nil {
		return nil, err
	}
	var result map[string]any
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &result) != nil {
		return nil, errors.New("registration result shape rejected")
	}
	return result, nil
}

func countSyncRegisterPG(t *testing.T, ctx context.Context, app *App, table string) int {
	t.Helper()
	if table != "streams" && table != "registrations" && table != "device_stores" {
		t.Fatal("owned registration table required")
	}
	var count int
	if app.DB.QueryRow(ctx, "SELECT count(*) FROM sync."+table).Scan(&count) != nil {
		t.Fatal("owned registration row count unavailable")
	}
	return count
}

func TestSyncRegisterPGKeepsOnePhysicalStoreAcrossSchemas(t *testing.T) {
	ctx, app, user := newSyncRegisterPGApp(t)
	store, requestID := newID(), newID()
	first, err := callSyncRegisterPG(app, "1", store, requestID)
	if err != nil || stringValue(first, "storeGeneration") != store || stringValue(first, "deviceId") != user.DeviceID || first["schemaVersion"] != nil {
		t.Fatal("schema-one physical store registration failed; private values suppressed")
	}
	second, err := callSyncRegisterPG(app, "2", store, newID())
	if err != nil || stringValue(second, "schemaVersion") != "2" || stringValue(second, "streamId") == stringValue(first, "streamId") || stringValue(second, "storeGeneration") != store || stringValue(second, "deviceId") != user.DeviceID {
		t.Fatal("schema negotiation changed the physical identity or failed to separate streams")
	}
	retry, err := callSyncRegisterPG(app, "1", store, requestID)
	if err != nil || stringValue(retry, "streamId") != stringValue(first, "streamId") {
		t.Fatal("the original schema-one stream changed on retry")
	}
	_, err = callSyncRegisterPG(app, "2", newID(), newID())
	var rejected *apiError
	if !errors.As(err, &rejected) || rejected.Status != 409 || rejected.Code != "STORE_GENERATION_CHANGED" {
		t.Fatal("a different local store crossed the schema fence")
	}
	if countSyncRegisterPG(t, ctx, app, "streams") != 2 || countSyncRegisterPG(t, ctx, app, "registrations") != 2 || countSyncRegisterPG(t, ctx, app, "device_stores") != 1 {
		t.Fatal("rejected registration created an orphan stream or physical store")
	}
	for _, schema := range []string{"1", "2"} {
		registered, err := app.syncRegistered(ctx, user, schema)
		if err != nil || registered.Store != store || registered.Schema != schema || !syncValidationID(registered.Stream) {
			t.Fatal("current registration lookup lost its schema/physical identity binding")
		}
	}
}

func TestSyncRegisterPGConcurrentSchemasCannotAdoptTwoStores(t *testing.T) {
	ctx, app, _ := newSyncRegisterPGApp(t)
	type outcome struct {
		body map[string]any
		err  error
	}
	start, done := make(chan struct{}), make(chan outcome, 2)
	for _, schema := range []string{"1", "2"} {
		schema := schema
		go func() {
			<-start
			body, err := callSyncRegisterPG(app, schema, newID(), newID())
			done <- outcome{body, err}
		}()
	}
	close(start)
	wins, denied := 0, 0
	for range 2 {
		result := <-done
		if result.err == nil {
			if !syncValidationID(stringValue(result.body, "storeGeneration")) {
				t.Fatal("winner omitted its physical store identity")
			}
			wins++
			continue
		}
		var rejected *apiError
		if errors.As(result.err, &rejected) && rejected.Status == 409 && rejected.Code == "STORE_GENERATION_CHANGED" {
			denied++
		} else {
			t.Fatal("concurrent registration produced an unexpected error; details redacted")
		}
	}
	if wins != 1 || denied != 1 || countSyncRegisterPG(t, ctx, app, "streams") != 1 || countSyncRegisterPG(t, ctx, app, "registrations") != 1 || countSyncRegisterPG(t, ctx, app, "device_stores") != 1 {
		t.Fatal("competing protocol schemas adopted multiple physical stores")
	}
	var schemas string
	if app.DB.QueryRow(ctx, "SELECT string_agg(schema_version,',' ORDER BY schema_version) FROM sync.registrations").Scan(&schemas) != nil || strings.Contains(schemas, ",") {
		t.Fatal("losing registration left a second schema row")
	}
}
