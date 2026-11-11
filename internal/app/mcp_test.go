package app

import (
	"acton/internal/act"
	"acton/internal/dockertest"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDesktopControl(t *testing.T) {
	dockertest.Require(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o750); err != nil {
		t.Fatal(err)
	}
	workflow := "name: CI\non: push\njobs:\n  check:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n"
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp()
	a := app.openProject(dir)
	a.settings.Secrets = []string{"TOKEN=private-fixture"}
	a.settings.ExtraArgs = "--secret=INLINE=extra-fixture"
	queue := make(chan func(), 16)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for action := range queue {
			action()
		}
	}()
	server := httptest.NewServer(app.mcpHandler(func(action func()) { queue <- action }))
	t.Cleanup(func() {
		server.Close()
		queue <- app.shutdown
		close(queue)
		<-finished
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil || len(listed.Tools) != 6 {
		t.Fatalf("tools: %v, %v", listed, err)
	}
	call := func(name string, args any, wantError bool) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError || len(result.Content) == 0 {
			t.Fatalf("%s: unexpected result %+v, content=%+v", name, result, result.Content[0])
		}
		return result.Content[0].(*mcp.TextContent).Text
	}
	current := call("get_settings", map[string]any{}, false)
	if strings.Contains(current, "private-fixture") || strings.Contains(current, "extra-fixture") {
		t.Fatal("MCP exposed inline secrets")
	}
	for _, patch := range []map[string]any{
		{"Env": []string{"CHANGED=value"}, "Unknown": true},
		{"Env": []string{"CHANGED=value"}, "DryRun": "wrong-type"},
		{"Platforms": []string{"invalid"}},
		{"ActrcFiles": []string{"fake"}},
		{"Dir": ""},
	} {
		call("update_settings", map[string]any{"patch": patch}, true)
		if got := call("get_settings", map[string]any{}, false); got != current {
			t.Fatal("failed update changed desktop settings")
		}
	}
	call("update_settings", map[string]any{"patch": map[string]any{"DryRun": true, "Arch": "linux/arm64", "ExtraArgs": "--no-cache-server"}}, false)
	call("start_run", map[string]any{}, false)
	call("start_run", map[string]any{}, true)
	call("update_settings", map[string]any{"patch": map[string]any{"Event": "release"}}, true)
	var run runSnapshot
	for {
		//nolint:musttag // The nested act model uses its established exported field names.
		if err := json.Unmarshal([]byte(call("get_run", map[string]any{}, false)), &run); err != nil {
			t.Fatal(err)
		}
		if run.Status != "Running" {
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("MCP run did not finish")
		}
	}
	if run.Status != "Succeeded" || run.ID != 1 || len(run.Output.Jobs) != 1 || run.Output.Jobs[0].Status != act.StatusSuccess {
		t.Fatalf("unexpected run: %+v", run)
	}
	call("start_run", map[string]any{}, false)
	var history struct {
		Runs []runSnapshot `json:"runs"`
	}
	//nolint:musttag // The nested act model uses its established exported field names.
	if err := json.Unmarshal([]byte(call("list_runs", map[string]any{}, false)), &history); err != nil || len(history.Runs) != 2 {
		t.Fatalf("session history: %+v, %v", history, err)
	}
	if saved := call("get_run", map[string]any{"id": 1}, false); !strings.Contains(saved, "Succeeded") {
		t.Fatal("previous run is unavailable")
	}
	call("get_run", map[string]any{"id": 999}, true)
	call("stop_run", map[string]any{}, false)
}

func TestMCPRejectsCrossOriginRequests(t *testing.T) {
	a := newApp()
	handler := a.mcpHandler(func(action func()) { action() })
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:7337/mcp", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status: %d", w.Code)
	}
	for _, addr := range []string{"0.0.0.0:7337", ":7337", "192.0.2.1:7337", "invalid"} {
		if err := a.serveMCP(addr); err == nil {
			t.Fatalf("accepted public or invalid address %q", addr)
		}
	}
}

func TestMCPRejectsRunWithoutWorkflows(t *testing.T) {
	app := newApp()
	a := app.openProject(t.TempDir())
	server := httptest.NewServer(app.mcpHandler(func(action func()) { action() }))
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "start_run", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "no GitHub Actions workflow files") {
		t.Fatalf("expected missing-workflow error: %+v", result)
	}
	if a.runner != nil || a.running {
		t.Fatal("MCP started a run without workflow files")
	}
}

func TestMCPQueuedRequestCancellation(t *testing.T) {
	a := newApp()
	var queued func()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	_, _, err := a.mcpCall(ctx, func(action func()) { queued = action }, func() (any, error) {
		called = true
		return struct{}{}, nil
	})
	queued()
	if err == nil || called {
		t.Fatal("cancelled request still changed state")
	}
	close(a.done)
	_, _, err = a.mcpCall(t.Context(), func(action func()) { action() }, func() (any, error) { return struct{}{}, nil })
	if err == nil {
		t.Fatal("request after shutdown did not return")
	}
}

func TestMCPDrivesMultipleProjects(t *testing.T) {
	dockertest.Require(t)
	app := newApp()
	first := app.openProject(newProject(t, "api"))
	secondDir := newProject(t, "web")
	server := httptest.NewServer(app.mcpHandler(func(action func()) { action() }))
	t.Cleanup(func() {
		server.Close()
		app.shutdown()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	call := func(name string, args map[string]any, wantError bool) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError || len(result.Content) == 0 {
			t.Fatalf("%s %v: unexpected result %+v", name, args, result)
		}
		return result.Content[0].(*mcp.TextContent).Text
	}
	call("get_settings", map[string]any{"project": secondDir}, true)
	call("start_run", map[string]any{"project": "relative/dir"}, true)
	call("start_run", map[string]any{"project": filepath.Join(t.TempDir(), "missing")}, true)
	// Updating an unopened project opens it without taking focus from the selected tab.
	call("update_settings", map[string]any{"project": secondDir, "patch": map[string]any{"DryRun": true}}, false)
	second := app.sessionFor(secondDir)
	if second == nil || !second.settings.DryRun || app.current != first || first.settings.DryRun {
		t.Fatalf("settings applied to the wrong project: second=%v current=%v", second, app.current)
	}
	call("update_settings", map[string]any{"patch": map[string]any{"DryRun": true}}, false)
	call("update_settings", map[string]any{"patch": map[string]any{"Dir": secondDir}}, true)
	call("start_run", map[string]any{"project": secondDir}, false)
	call("start_run", map[string]any{}, false)
	if !first.running || !second.running {
		t.Fatal("runs for different projects did not overlap")
	}
	call("start_run", map[string]any{"project": secondDir}, true)
	for _, dir := range []string{first.settings.Dir, secondDir} {
		for {
			var run runSnapshot
			//nolint:musttag // The nested act model uses its established exported field names.
			if err := json.Unmarshal([]byte(call("get_run", map[string]any{"project": dir}, false)), &run); err != nil {
				t.Fatal(err)
			}
			if run.Status != "Running" {
				if run.Status != "Succeeded" || run.Project != dir {
					t.Fatalf("run for %s: %+v", dir, run)
				}
				break
			}
			select {
			case <-time.After(10 * time.Millisecond):
			case <-ctx.Done():
				t.Fatal("MCP run did not finish")
			}
		}
	}
	var all struct {
		Runs []runSnapshot `json:"runs"`
	}
	//nolint:musttag // The nested act model uses its established exported field names.
	if err := json.Unmarshal([]byte(call("list_runs", map[string]any{}, false)), &all); err != nil || len(all.Runs) != 2 || all.Runs[0].ID == all.Runs[1].ID {
		t.Fatalf("runs across projects: %+v, %v", all, err)
	}
	//nolint:musttag // The nested act model uses its established exported field names.
	if err := json.Unmarshal([]byte(call("list_runs", map[string]any{"project": secondDir}, false)), &all); err != nil || len(all.Runs) != 1 || all.Runs[0].Project != secondDir {
		t.Fatalf("project filter: %+v, %v", all, err)
	}
	if saved := call("get_run", map[string]any{"id": second.runID}, false); !strings.Contains(saved, secondDir) {
		t.Fatal("run lookup by id ignored its project")
	}
	call("stop_run", map[string]any{"project": secondDir}, false)
	app.current = nil
	call("get_settings", map[string]any{}, true) // Two tabs open and none selected: the project must be named.
}
