package act

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nektos/act/pkg/artifactcache"
	"github.com/nektos/act/pkg/artifacts"
	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/container"
	"github.com/nektos/act/pkg/gh"
	"github.com/nektos/act/pkg/model"
	actrunner "github.com/nektos/act/pkg/runner"
	"github.com/sirupsen/logrus"
)

// act uses a process-wide logger and reads DOCKER_HOST from the environment.
var executionMu sync.Mutex

// Runner executes one embedded act plan and buffers JSON logs for the UI.
type Runner struct {
	mu                  sync.Mutex
	pending             []string
	cancel, forceCancel context.CancelFunc
	timer               *time.Timer
	finished            chan struct{}
	done                bool
	exitErr             error
	cleanupErr          error
	onFlush             func()
	level               logrus.Level
	insecureSecrets     bool
}

// Start validates flags, then loads and executes the workflow off the UI thread.
// Callbacks run on workers and must schedule UI updates.
func (r *Runner) Start(dir string, args []string, onFlush, onDone func()) error {
	o, err := parseOptions(dir, args)
	if err != nil {
		return err
	}
	if !executionMu.TryLock() {
		return errors.New("an act run is already active")
	}
	r.mu.Lock()
	if r.finished != nil {
		r.mu.Unlock()
		executionMu.Unlock()
		return errors.New("runner has already been started")
	}
	ctx, forceCancel := context.WithCancel(context.Background())
	jobCtx, cancel := context.WithCancel(ctx)
	ctx = common.WithJobCancelContext(ctx, jobCtx)
	r.cancel, r.forceCancel = cancel, forceCancel
	r.finished = make(chan struct{})
	r.onFlush = onFlush
	r.insecureSecrets = o.config.InsecureSecrets
	r.level = logrus.InfoLevel
	if o.verbose {
		r.level = logrus.DebugLevel
	}
	r.mu.Unlock()
	go func() {
		err := r.execute(ctx, o, dir)
		r.mu.Lock()
		switch {
		case r.exitErr != nil:
			err = r.exitErr
		case jobCtx.Err() != nil:
			err = errors.Join(context.Canceled, err)
		}
		err = errors.Join(err, r.cleanupErr)
		r.exitErr = err
		if r.timer != nil {
			r.timer.Stop()
		}
		cancel()
		forceCancel()
		r.done = true
		executionMu.Unlock()
		close(r.finished)
		r.mu.Unlock()
		onDone()
	}()
	return nil
}

func (r *Runner) execute(ctx context.Context, o *options, dir string) (err error) {
	logger := r.WithJobLogger()
	// Some act packages log outside the context; restore the host logger after the run.
	standard := logrus.StandardLogger()
	output, formatter, level, exit := standard.Out, standard.Formatter, standard.GetLevel(), standard.ExitFunc
	standard.SetOutput(r)
	standard.SetFormatter(&logFormatter{})
	standard.SetLevel(r.level)
	standard.ExitFunc = func(int) { r.fail(errors.New("act failed; see runner output")) }
	defer func() {
		standard.SetOutput(output)
		standard.SetFormatter(formatter)
		standard.SetLevel(level)
		standard.ExitFunc = exit
	}()
	ctx = common.WithLogger(ctx, logger)
	ctx = actrunner.WithJobLoggerFactory(ctx, r)
	ctx = common.WithDryrun(ctx, o.dryrun)
	if err := o.loadValues(); err != nil {
		return err
	}
	c := &o.config
	if _, exists := c.Secrets["GITHUB_TOKEN"]; !exists {
		tokenCtx, cancel := common.EarlyCancelContext(ctx)
		c.Token, _ = gh.GetToken(tokenCtx, c.Workdir)
		cancel()
		c.Secrets["GITHUB_TOKEN"] = c.Token
	}
	planner, err := model.NewWorkflowPlanner(o.workflows, o.noRecurse, o.strict)
	if err != nil {
		return fmt.Errorf("load workflows: %w", err)
	}
	var plan *model.Plan
	if o.job != "" {
		plan, err = planner.PlanJob(o.job)
	} else {
		plan, err = planner.PlanEvent(c.EventName)
	}
	if err != nil {
		return fmt.Errorf("plan workflows: %w", err)
	}
	if plan == nil || len(plan.Stages) == 0 {
		return errors.New("no jobs match the selected workflow, event, and job")
	}
	if err := common.JobCancelContext(ctx).Err(); err != nil {
		return fmt.Errorf("plan cancelled: %w", err)
	}
	socket, socketErr := container.GetSocketAndHost(c.ContainerDaemonSocket)
	if socketErr != nil {
		logger.Warnf("Docker connection: %v", socketErr)
	} else {
		c.ContainerDaemonSocket = socket.Socket
		previous, existed := os.LookupEnv("DOCKER_HOST")
		if err := os.Setenv("DOCKER_HOST", socket.Host); err != nil {
			return fmt.Errorf("configure Docker: %w", err)
		}
		defer func() {
			var restoreErr error
			if existed {
				restoreErr = os.Setenv("DOCKER_HOST", previous)
			} else {
				restoreErr = os.Unsetenv("DOCKER_HOST")
			}
			err = errors.Join(err, restoreErr)
		}()
	}
	if !o.dryrun {
		id, idErr := projectID(dir)
		if idErr != nil {
			return idErr
		}
		cli, clientErr := container.GetDockerClient(ctx)
		if clientErr != nil {
			return fmt.Errorf("connect Docker: %w", clientErr)
		}
		defer func() { err = errors.Join(err, cli.Close()) }()
		if err := cleanupResources(ctx, cli, id); err != nil {
			return fmt.Errorf("clean previous run: %w", err)
		}
		host, closeProxy, proxyErr := startDockerProxy(ctx, cli, id)
		if proxyErr != nil {
			return proxyErr
		}
		defer closeProxy()
		for _, key := range []string{"DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"} {
			previous, existed := os.LookupEnv(key)
			defer func() {
				var restoreErr error
				if existed {
					restoreErr = os.Setenv(key, previous)
				} else {
					restoreErr = os.Unsetenv(key)
				}
				err = errors.Join(err, restoreErr)
			}()
			if key == "DOCKER_HOST" {
				err = os.Setenv(key, host)
			} else {
				err = os.Unsetenv(key)
			}
			if err != nil {
				return fmt.Errorf("configure Docker proxy: %w", err)
			}
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if cleanupErr := cleanupResources(cleanupCtx, cli, id); cleanupErr != nil {
				r.mu.Lock()
				r.cleanupErr = fmt.Errorf("clean completed run: %w", cleanupErr)
				r.mu.Unlock()
			}
		}()
	}
	engine, err := actrunner.New(c)
	if err != nil {
		return fmt.Errorf("configure act: %w", err)
	}
	if !o.dryrun {
		if c.ArtifactServerAddr == "" && c.ArtifactServerPath != "" {
			ip := common.GetOutboundIP()
			if ip == nil {
				return errors.New("cannot determine artifact server address; set --artifact-server-addr")
			}
			c.ArtifactServerAddr = ip.String()
		}
		if c.ArtifactServerPath != "" {
			if c.ArtifactServerAuthKey, err = artifacts.NewAuthKey(); err != nil {
				return fmt.Errorf("configure artifact server: %w", err)
			}
		}
		stopArtifacts := artifacts.Serve(ctx, c.ArtifactServerPath, c.ArtifactServerAddr, c.ArtifactServerPort, c.ArtifactServerAuthKey)
		defer stopArtifacts()
		if !o.noCacheServer && c.Env["ACTIONS_CACHE_URL"] == "" {
			cache, cacheErr := artifactcache.StartHandler(o.cachePath, o.cacheURL, o.cacheAddr, o.cachePort, logger)
			if cacheErr != nil {
				return fmt.Errorf("start cache server: %w", cacheErr)
			}
			defer func() { err = errors.Join(err, cache.Close()) }()
			c.Env["ACTIONS_CACHE_URL"] = cache.ExternalURL() + "/"
		}
	}
	if err := engine.NewPlanExecutor(plan)(ctx); err != nil {
		return fmt.Errorf("run workflows: %w", err)
	}
	return nil
}

// WithJobLogger gives each job its own logger; act wraps its formatter to mask
// secrets and ::add-mask:: values before writing JSON into the shared buffer.
func (r *Runner) WithJobLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(r)
	logger.SetFormatter(&logFormatter{masked: !r.insecureSecrets})
	logger.SetLevel(r.level)
	// An artifact server bind failure must cancel the run, never exit the GUI.
	logger.ExitFunc = func(int) { r.fail(errors.New("act server failed; see runner output")) }
	return logger
}

func (r *Runner) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.exitErr = err
	r.forceCancel()
}

// Write accepts complete JSON records from act's loggers, without a pipe or
// scanner size limit. JSON escapes newlines inside workflow output.
func (r *Runner) Write(p []byte) (int, error) {
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	r.mu.Lock()
	if !r.done {
		r.pending = append(r.pending, lines...)
	}
	r.mu.Unlock()
	r.onFlush()
	return len(p), nil
}

// Stop first cancels jobs, allowing post steps and container cleanup, then
// cancels the entire execution context after the grace period.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || r.cancel == nil || r.timer != nil {
		return
	}
	r.cancel()
	r.timer = time.AfterFunc(4*time.Second, r.forceCancel)
}

// Wait waits for execution and cleanup to finish before the app exits.
func (r *Runner) Wait() {
	r.mu.Lock()
	finished := r.finished
	r.mu.Unlock()
	if finished != nil {
		<-finished
	}
}

// Drain returns buffered records and the execution status atomically.
func (r *Runner) Drain() (lines []string, done bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines, r.pending = r.pending, nil
	return lines, r.done, r.exitErr
}

// act masks messages but leaves workflow-command arguments in JSON fields.
// Those unused diagnostic fields can contain secrets, including add-mask values.
type logFormatter struct {
	logrus.JSONFormatter
	masked bool
}

func (f *logFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Only act's job logger applies its secret masker before our formatter.
	entry.Data["msg_masked"] = f.masked && entry.Data["job"] != nil
	delete(entry.Data, "arg")
	delete(entry.Data, "raw")
	delete(entry.Data, "kvPairs")
	data, err := f.JSONFormatter.Format(entry)
	if err != nil {
		return nil, fmt.Errorf("format act log: %w", err)
	}
	return data, nil
}
