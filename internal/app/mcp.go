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
	"path/filepath"
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

func (s *Session) snapshot() runSnapshot {
	return runSnapshot{ID: s.runID, Project: s.runDir, Started: s.started, Status: s.overallStatus().String(), Error: s.runErr, Output: s.run}
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

// mcpSession finds the session a tool call targets: the named project, or the
// selected tab. With open set, a project that is not open yet gets a tab
// without taking focus.
func (a *App) mcpSession(project string, open bool) (*Session, error) {
	if project == "" {
		switch {
		case a.current != nil:
			return a.current, nil
		case len(a.sessions) == 1:
			return a.sessions[0], nil
		}
		return nil, errors.New("no project is selected: pass the project directory")
	}
	if !filepath.IsAbs(project) {
		return nil, errors.New("project must be an absolute directory path")
	}
	if s := a.sessionFor(project); s != nil {
		return s, nil
	}
	if !open {
		return nil, errors.New("project is not open: call update_settings or start_run with it first")
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return nil, errors.New("project must be an existing directory")
	}
	return a.addSession(project), nil
}

// projectInput selects the project a tool acts on.
type projectInput struct {
	Project string `json:"project,omitempty" jsonschema:"Absolute project directory; defaults to the selected project tab."`
}

func (a *App) mcpHandler(dispatch func(func())) http.Handler {
	s := mcp.NewServer(&mcp.Implementation{Name: "acton", Version: "0.1.0"}, nil)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(s, &mcp.Tool{Name: "list_runs", Description: "List runs from this desktop session across all projects, including current runs. Pass project to list one project's runs.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				runs := slices.Clone(a.history)
				for _, session := range a.sessions {
					if session.running {
						session.pump()
					}
					if session.runID != 0 {
						runs = append(runs, session.snapshot())
					}
				}
				if in.Project != "" {
					want := canonicalDir(in.Project)
					runs = slices.DeleteFunc(runs, func(run runSnapshot) bool { return canonicalDir(run.Project) != want })
				}
				slices.SortFunc(runs, func(x, y runSnapshot) int { return x.ID - y.ID })
				for i := range runs {
					runs[i].Output = act.Run{}
				}
				return map[string]any{"runs": runs}, nil
			})
		})
	type runInput struct {
		Project string `json:"project,omitempty" jsonschema:"Absolute project directory; defaults to the selected project tab. Ignored when id is set."`
		ID      int    `json:"id,omitempty" jsonschema:"Run ID from list_runs; omit for the project's current or latest run."`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "get_run", Description: "Read a run's jobs, steps, masked output, and runner errors. Status values in output: 0 pending, 1 running, 2 success, 3 failure, 4 skipped, 5 cancelled.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in runInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				var project string
				if in.ID == 0 {
					session, err := a.mcpSession(in.Project, false)
					if err != nil {
						return nil, err
					}
					if session.running {
						session.pump()
					}
					if session.runID != 0 {
						return session.snapshot(), nil
					}
					project = canonicalDir(session.settings.Dir)
				}
				for _, session := range a.sessions {
					if session.running {
						session.pump()
					}
					if session.runID != 0 && session.runID == in.ID {
						return session.snapshot(), nil
					}
				}
				for _, run := range slices.Backward(a.history) {
					if run.ID == in.ID || (in.ID == 0 && canonicalDir(run.Project) == project) {
						return run, nil
					}
				}
				return nil, errors.New("run not found")
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "start_run", Description: "Start a project's workflows using its current settings. Runs for different projects can overlap; a project rejects a second concurrent run. Workflow commands can modify files and use Docker or the network."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, any, error) {
			//nolint:contextcheck // A started workflow outlives this HTTP request; stop_run cancels it explicitly.
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				session, err := a.mcpSession(in.Project, true)
				if err != nil {
					return nil, err
				}
				if session.running {
					return nil, errors.New("wait for the project's active run to finish")
				}
				if len(session.workflows) == 0 {
					return nil, errors.New("no GitHub Actions workflow files found in this project")
				}
				session.startRun()
				if !session.running {
					return nil, errors.New(session.runErr)
				}
				return session.snapshot(), nil
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "stop_run", Description: "Cancel a project's active run, allowing post steps and cleanup."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				session, err := a.mcpSession(in.Project, false)
				if err != nil {
					return nil, err
				}
				session.stopRun()
				return map[string]bool{"stopping": session.running}, nil
			})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_settings", Description: "Read a project's current desktop settings. Secret values are redacted; omit those fields when updating to preserve them.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				session, err := a.mcpSession(in.Project, false)
				if err != nil {
					return nil, err
				}
				return session.settingsSnapshot(), nil
			})
		})
	type settingsInput struct {
		Project string                     `json:"project,omitempty"`
		Patch   map[string]json.RawMessage `json:"patch" jsonschema:"Fields to change: Dir, Workflows, Event, Job, Platforms, Arch, Secrets, SecretFile, Env, EnvFile, Vars, VarFile, Inputs, EventPayload, DryRun, Verbose, Pull, Reuse, Bind, Quiet, ExtraArgs. Omitted fields are preserved. Changing Dir loads that project's defaults first."`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "update_settings", Description: "Update a project's desktop settings in memory, opening its tab if needed. Rejects invalid flags and changes while the project's run is active. Use the desktop Save button to persist settings.", InputSchema: map[string]any{
		"type": "object", "required": []string{"patch"}, "additionalProperties": false,
		"properties": map[string]any{
			"project": map[string]any{"type": "string", "description": "Absolute project directory; defaults to the selected project tab."},
			"patch":   map[string]any{"type": "object", "additionalProperties": true},
		},
	}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in settingsInput) (*mcp.CallToolResult, any, error) {
			return a.mcpCall(ctx, dispatch, func() (any, error) {
				session, err := a.mcpSession(in.Project, true)
				if err != nil {
					return nil, err
				}
				if err := session.updateSettings(in.Patch); err != nil {
					return nil, err
				}
				return session.settingsSnapshot(), nil
			})
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.NewCrossOriginProtection().Handler(handler))
	return mux
}

func (s *Session) settingsSnapshot() settings.Settings {
	snapshot := s.settings
	snapshot.Secrets = slices.Clone(snapshot.Secrets)
	for i, secret := range snapshot.Secrets {
		name, _, _ := strings.Cut(secret, "=")
		snapshot.Secrets[i] = name + "=••••••••"
	}
	if args, err := shellquote.Split(snapshot.ExtraArgs); err == nil {
		redactArgs(args)
		snapshot.ExtraArgs = shellquote.Join(args...)
	} else {
		snapshot.ExtraArgs = "<invalid extra arguments>"
	}
	return snapshot
}

func (s *Session) updateSettings(patch map[string]json.RawMessage) error {
	if s.running {
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
	candidate := s.settings
	if changes.Dir != "" && changes.Dir != candidate.Dir {
		if other := s.app.sessionFor(changes.Dir); other != nil && other != s {
			return errors.New("dir is already open in another project tab")
		}
		candidate = *settings.Default(changes.Dir)
		candidate.LoadActrc()
	}
	candidate.Platforms = slices.Clone(candidate.Platforms)
	candidate.Workflows = slices.Clone(candidate.Workflows)
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
	if candidate.Dir != s.settings.Dir {
		s.clearRun()
	}
	s.settings = candidate
	s.eventSel, s.eventOther = candidate.Event, candidate.Event
	if !contains(workflowEvents, s.eventSel) {
		s.eventSel = "custom"
	}
	s.fileErr = ""
	s.rescanWorkflows()
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
