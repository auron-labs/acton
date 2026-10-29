package app

import (
	"acton/internal/act"
	"acton/internal/settings"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type runSnapshot struct {
	ID      int       `json:"id"`
	Project string    `json:"project"`
	Started time.Time `json:"started"`
	Status  string    `json:"status"`
	Error   string    `json:"error,omitempty"`
	Output  act.Run   `json:"output"`
}

func (a *App) snapshot() runSnapshot {
	return runSnapshot{ID: a.runID, Project: a.runDir, Started: a.started, Status: a.overallStatus().String(), Error: a.runErr, Output: a.run}
}

// mcpCall serializes both state access and JSON encoding on the UI thread.
func (a *App) mcpCall(ctx context.Context, dispatch func(func()), action func() (any, error)) (*mcp.CallToolResult, any, error) {
	type response struct {
		data []byte
		err  error
	}
	result := make(chan response, 1)
	dispatch(func() {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		default:
		}
		value, err := action()
		if err != nil {
			result <- response{err: err}
			return
		}
		data, err := json.Marshal(value)
		result <- response{data: data, err: err}
	})
	select {
	case r := <-result:
		if r.err != nil {
			return nil, nil, r.err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(r.data)}}, StructuredContent: json.RawMessage(r.data)}, nil, nil
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("MCP request cancelled: %w", ctx.Err())
	case <-a.done:
		return nil, nil, errors.New("acton is closing")
	}
}

func (a *App) mcpHandler(dispatch func(func())) http.Handler {
	s := mcp.NewServer(&mcp.Implementation{Name: "acton", Version: "0.1.0"}, nil)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(s, &mcp.Tool{Name: "list_runs", Description: "List runs from this desktop session, including the current run.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				if a.running {
					a.pump()
				}
				runs := slices.Clone(a.history)
				if a.runID != 0 {
					runs = append(runs, a.snapshot())
				}
				for i := range runs {
					runs[i].Output = act.Run{}
				}
				return map[string]any{"runs": runs}, nil
			})
		})
	type runInput struct {
		ID int `json:"id,omitempty" jsonschema:"Run ID from list_runs; omit for the current or latest run."`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "get_run", Description: "Read a run's jobs, steps, masked output, and runner errors. Status values in output: 0 pending, 1 running, 2 success, 3 failure, 4 skipped, 5 cancelled.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in runInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				if a.running {
					a.pump()
				}
				if a.runID != 0 && (in.ID == 0 || in.ID == a.runID) {
					return a.snapshot(), nil
				}
				for _, run := range slices.Backward(a.history) {
					if in.ID == 0 || run.ID == in.ID {
						return run, nil
					}
				}
				return nil, errors.New("run not found")
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "start_run", Description: "Start a workflow using the desktop's current settings. Rejects concurrent runs. Workflow commands can modify files and use Docker or the network."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			//nolint:contextcheck // A started workflow outlives this HTTP request; stop_run cancels it explicitly.
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				if a.running || a.settings.Dir == "" {
					return nil, errors.New("choose a project and wait for any active run to finish")
				}
				if len(a.workflows) == 0 {
					return nil, errors.New("no GitHub Actions workflow files found in this project")
				}
				a.startRun()
				if !a.running {
					return nil, errors.New(a.runErr)
				}
				return a.snapshot(), nil
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "stop_run", Description: "Cancel the active run, allowing post steps and cleanup."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				a.stopRun()
				return map[string]bool{"stopping": a.running}, nil
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_settings", Description: "Read current desktop settings. Secret values are redacted; omit those fields when updating to preserve them.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) { return a.settingsSnapshot(), nil })
		})
	type settingsInput struct {
		Patch map[string]json.RawMessage `json:"patch" jsonschema:"Fields to change: Dir, Workflow, Event, Job, Platforms, Arch, Secrets, SecretFile, Env, EnvFile, Vars, VarFile, Inputs, EventPayload, DryRun, Verbose, Pull, Reuse, Bind, Quiet, ExtraArgs. Omitted fields are preserved. Changing Dir loads that project's defaults first."`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "update_settings", Description: "Update desktop settings in memory. Rejects invalid flags and changes while a run is active. Use the desktop Save button to persist settings.", InputSchema: map[string]any{
		"type": "object", "required": []string{"patch"}, "additionalProperties": false,
		"properties": map[string]any{"patch": map[string]any{"type": "object", "additionalProperties": true}},
	}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in settingsInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				if err := a.updateSettings(in.Patch); err != nil {
					return nil, err
				}
				return a.settingsSnapshot(), nil
			})
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.NewCrossOriginProtection().Handler(handler))
	return mux
}

func (a *App) settingsSnapshot() settings.Settings {
	s := a.settings
	s.Secrets = slices.Clone(s.Secrets)
	for i, secret := range s.Secrets {
		name, _, _ := strings.Cut(secret, "=")
		s.Secrets[i] = name + "=••••••••"
	}
	if args, err := shellquote.Split(s.ExtraArgs); err == nil {
		redactArgs(args)
		s.ExtraArgs = shellquote.Join(args...)
	} else {
		s.ExtraArgs = "<invalid extra arguments>"
	}
	return s
}

func (a *App) updateSettings(patch map[string]json.RawMessage) error {
	if a.running {
		return errors.New("cannot change settings while a run is active")
	}
	for name, value := range patch {
		if strings.EqualFold(name, "ActrcFiles") {
			return errors.New("ActrcFiles is read-only")
		}
		if string(value) == "null" || bytes.Contains(value, []byte("••••••••")) {
			return errors.New("omit unchanged fields; provide actual values instead of null or redacted secrets")
		}
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("encode settings patch: %w", err)
	}
	var changes settings.Settings
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	//nolint:musttag // The existing settings wire format intentionally uses exported Go field names.
	if err := decoder.Decode(&changes); err != nil {
		// Decoder errors can include user-supplied values; never echo secrets.
		return errors.New("invalid settings patch: check field names and value types")
	}
	candidate := a.settings
	if changes.Dir != "" && changes.Dir != candidate.Dir {
		candidate = *settings.Default(changes.Dir)
		candidate.LoadActrc()
	}
	candidate.Platforms = slices.Clone(candidate.Platforms)
	candidate.Secrets = slices.Clone(candidate.Secrets)
	candidate.Env = slices.Clone(candidate.Env)
	candidate.Vars = slices.Clone(candidate.Vars)
	candidate.Inputs = slices.Clone(candidate.Inputs)
	//nolint:musttag // Preserve the existing settings field names in the MCP wire format.
	if err := json.Unmarshal(data, &candidate); err != nil {
		return errors.New("invalid settings patch")
	}
	info, err := os.Stat(candidate.Dir)
	if err != nil || !info.IsDir() {
		return errors.New("dir must be an existing project directory")
	}
	args, err := candidate.Args()
	if err != nil {
		return fmt.Errorf("validate settings: %w", err)
	}
	if err := act.Validate(candidate.Dir, args); err != nil {
		return errors.New("invalid act flags in settings")
	}
	if candidate.Dir != a.settings.Dir {
		a.clearRun()
	}
	a.settings = candidate
	a.eventSel, a.eventOther = candidate.Event, candidate.Event
	if !contains(workflowEvents, a.eventSel) {
		a.eventSel = "custom"
	}
	a.fileErr = ""
	a.rescanWorkflows()
	return nil
}

func (a *App) serveMCP(addr string) error {
	endpoint, err := netip.ParseAddrPort(addr)
	if err != nil || !endpoint.Addr().IsLoopback() {
		return errors.New("MCP address must be a numeric loopback IP and port, such as 127.0.0.1:7337")
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for MCP: %w", err)
	}
	a.mcpServer = &http.Server{Handler: a.mcpHandler(a.win.Update), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := a.mcpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("acton MCP: %v", err)
		}
	}()
	log.Printf("acton MCP listening at http://%s/mcp", listener.Addr())
	return nil
}
