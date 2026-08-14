package mcpadapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const mcpTestInstanceID = "11111111-1111-4111-8111-111111111111"

type mcpTestTargetClient struct {
	get          func(string) ([]byte, error)
	getContext   func(context.Context, string) ([]byte, error)
	openActivity func(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error)
	activityErr  error
}

func assertJSONGolden(t *testing.T, name string, value any) {
	t.Helper()
	actual, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if string(actual)+"\n" != string(expected) {
		t.Fatalf("%s changed:\n%s", name, actual)
	}
}

func (*mcpTestTargetClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID: mcpTestInstanceID, ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt: time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC), LiveMonitoringAvailable: true,
	}, nil
}

func (client *mcpTestTargetClient) Get(ctx context.Context, endpoint string, _ int64, _ applicationclient.Credential) ([]byte, string, error) {
	var body []byte
	var err error
	if client.getContext != nil {
		body, err = client.getContext(ctx, endpoint)
	} else {
		body, err = client.get(endpoint)
	}
	return body, mcpTestInstanceID, err
}

func (client *mcpTestTargetClient) OpenActivity(ctx context.Context, instanceID, afterCursor string, credential applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	if client.openActivity != nil {
		return client.openActivity(ctx, instanceID, afterCursor, credential)
	}
	return nil, client.activityErr
}

func (*mcpTestTargetClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return nil, nil
}

func (*mcpTestTargetClient) Close() {}

func newMCPTestOptions(t *testing.T, get func(string) ([]byte, error)) ServerOptions {
	t.Helper()
	return newMCPTestOptionsWithClient(t, &mcpTestTargetClient{get: get})
}

func newMCPTestOptionsWithClient(t *testing.T, client *mcpTestTargetClient) ServerOptions {
	t.Helper()
	targetContext, err := target.New(
		func(applicationclient.Address) (target.ProbeClient, error) { return client, nil },
		func() (target.ScopeID, error) { return "scope-1", nil },
		func() time.Time { return time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(targetContext.Close)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}, key: "mcp-secret"}
	return ServerOptions{
		Port: 7345, Credentials: credentials, Tracker: NewTracker(),
		Status: func() consolecore.StatusSnapshot { return targetContext.Snapshot().Status },
		Target: targetContext, Observability: observability.New(),
		Now: func() time.Time { return time.Date(2026, 8, 13, 21, 0, 0, 123, time.UTC) },
	}
}
