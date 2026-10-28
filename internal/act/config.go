package act

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/nektos/act/pkg/runner"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// options translates the existing settings/extra-args contract into act's Go API.
type options struct {
	config                                                           runner.Config
	workflows, job                                                   string
	dryrun, verbose, noRecurse, strict                               bool
	envFile, secretFile, varFile, inputFile                          string
	filesRequired                                                    [4]bool
	env, secrets, vars, inputs, platforms, matrix, localRepositories []string
	noCacheServer                                                    bool
	cachePath, cacheAddr, cacheURL                                   string
	cachePort                                                        uint16
	newActionCache                                                   bool
}

func parseOptions(dir string, args []string) (*options, error) {
	o := &options{}
	c := &o.config
	f := pflag.NewFlagSet("embedded act", pflag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVarP(&c.Workdir, "directory", "C", ".", "")
	f.StringVarP(&o.workflows, "workflows", "W", ".github/workflows", "")
	f.StringVarP(&o.job, "job", "j", "", "")
	f.StringVarP(&c.Actor, "actor", "a", "nektos/act", "")
	f.StringVarP(&c.EventPath, "eventpath", "e", "", "")
	f.StringVar(&c.DefaultBranch, "defaultbranch", "", "")
	f.StringArrayVarP(&o.platforms, "platform", "P", nil, "")
	f.StringArrayVarP(&o.secrets, "secret", "s", nil, "")
	f.StringArrayVar(&o.env, "env", nil, "")
	f.StringArrayVar(&o.vars, "var", nil, "")
	f.StringArrayVar(&o.inputs, "input", nil, "")
	f.StringVar(&o.envFile, "env-file", ".env", "")
	f.StringVar(&o.secretFile, "secret-file", ".secrets", "")
	f.StringVar(&o.varFile, "var-file", ".vars", "")
	f.StringVar(&o.inputFile, "input-file", ".input", "")
	f.BoolVarP(&o.dryrun, "dryrun", "n", false, "")
	f.BoolVarP(&o.verbose, "verbose", "v", false, "")
	f.BoolVarP(&c.ForcePull, "pull", "p", true, "")
	f.BoolVar(&c.ForceRebuild, "rebuild", true, "")
	f.BoolVarP(&c.ReuseContainers, "reuse", "r", false, "")
	f.BoolVarP(&c.BindWorkdir, "bind", "b", false, "")
	quiet := f.BoolP("quiet", "q", false, "")
	f.BoolVar(&o.noRecurse, "no-recurse", false, "")
	f.BoolVar(&o.strict, "strict", false, "")
	f.Bool("json", true, "") // The GUI always uses JSON, even if extra args disable it.
	f.BoolVar(&c.LogPrefixJobID, "log-prefix-job-id", false, "")
	f.BoolVar(&c.InsecureSecrets, "insecure-secrets", false, "")
	f.BoolVar(&c.Privileged, "privileged", false, "")
	f.StringVar(&c.UsernsMode, "userns", "", "")
	f.BoolVar(&c.UseGitIgnore, "use-gitignore", true, "")
	f.StringVar(&c.ContainerArchitecture, "container-architecture", "", "")
	f.StringVar(&c.ContainerDaemonSocket, "container-daemon-socket", "", "")
	f.StringVar(&c.ContainerOptions, "container-options", "", "")
	f.StringVar(&c.GitHubInstance, "github-instance", "github.com", "")
	f.StringVar(&c.RemoteName, "remote-name", "origin", "")
	f.BoolVar(&c.NoSkipCheckout, "no-skip-checkout", false, "")
	f.StringArrayVar(&c.ContainerCapAdd, "container-cap-add", nil, "")
	f.StringArrayVar(&c.ContainerCapDrop, "container-cap-drop", nil, "")
	f.BoolVar(&c.AutoRemove, "rm", false, "")
	f.StringArrayVar(&c.ReplaceGheActionWithGithubCom, "replace-ghe-action-with-github-com", nil, "")
	f.StringVar(&c.ReplaceGheActionTokenWithGithubCom, "replace-ghe-action-token-with-github-com", "", "")
	f.StringArrayVar(&o.matrix, "matrix", nil, "")
	network := f.String("network", "host", "")
	f.IntVar(&c.ConcurrentJobs, "concurrent-jobs", 0, "")
	f.BoolVar(&c.ActionOfflineMode, "action-offline-mode", false, "")
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cache directory: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	f.StringVar(&c.ActionCacheDir, "action-cache-path", filepath.Join(cacheHome, "act"), "")
	f.BoolVar(&o.newActionCache, "use-new-action-cache", false, "")
	f.StringArrayVar(&o.localRepositories, "local-repository", nil, "")
	f.BoolVar(&o.noCacheServer, "no-cache-server", false, "")
	f.StringVar(&o.cachePath, "cache-server-path", filepath.Join(cacheHome, "actcache"), "")
	f.StringVar(&o.cacheAddr, "cache-server-addr", "", "")
	f.StringVar(&o.cacheURL, "cache-server-external-url", "", "")
	f.Uint16Var(&o.cachePort, "cache-server-port", 0, "")
	f.StringVar(&c.ArtifactServerPath, "artifact-server-path", "", "")
	f.StringVar(&c.ArtifactServerAddr, "artifact-server-addr", "", "")
	f.StringVar(&c.ArtifactServerPort, "artifact-server-port", "34567", "")
	if err := f.Parse(args); err != nil {
		return nil, fmt.Errorf("embedded act flags: %w", err)
	}
	if f.NArg() > 1 {
		return nil, errors.New("expected a single event name")
	}
	c.EventName = "push"
	if f.NArg() == 1 {
		c.EventName = f.Arg(0)
	}
	c.JSONLogger, c.LogOutput = true, !*quiet
	c.ForcePull = c.ForcePull && !c.ActionOfflineMode
	c.ForceRebuild = c.ForceRebuild && !c.ActionOfflineMode
	c.ContainerNetworkMode = dockercontainer.NetworkMode(*network)
	if c.ConcurrentJobs < 0 {
		return nil, errors.New("concurrent-jobs must not be negative")
	}
	for i, name := range []string{"env-file", "input-file", "secret-file", "var-file"} {
		o.filesRequired[i] = f.Changed(name)
	}
	var err error
	c.Workdir, err = filepath.Abs(resolvePath(dir, c.Workdir))
	if err != nil {
		return nil, fmt.Errorf("working directory: %w", err)
	}
	o.workflows = resolvePath(c.Workdir, o.workflows)
	c.EventPath = resolvePath(c.Workdir, c.EventPath)
	c.ActionCacheDir = resolvePath(c.Workdir, c.ActionCacheDir)
	c.ArtifactServerPath = resolvePath(c.Workdir, c.ArtifactServerPath)
	o.cachePath = resolvePath(c.Workdir, o.cachePath)
	c.Platforms = map[string]string{
		"ubuntu-latest": "node:16-buster-slim",
		"ubuntu-22.04":  "node:16-bullseye-slim",
		"ubuntu-20.04":  "node:16-buster-slim",
		"ubuntu-18.04":  "node:16-buster-slim",
	}
	for _, platform := range o.platforms {
		name, image, ok := strings.Cut(platform, "=")
		if !ok || name == "" || image == "" {
			return nil, errors.New("platform must have the form runner=image")
		}
		c.Platforms[strings.ToLower(name)] = image
	}
	c.Matrix = make(map[string]map[string]bool)
	for _, matrix := range o.matrix {
		key, value, ok := strings.Cut(matrix, ":")
		if !ok || key == "" || value == "" {
			return nil, errors.New("matrix must have the form key:value")
		}
		if c.Matrix[key] == nil {
			c.Matrix[key] = make(map[string]bool)
		}
		c.Matrix[key][value] = true
	}
	if o.newActionCache || len(o.localRepositories) > 0 {
		cache := runner.GoGitActionCache{Path: c.ActionCacheDir}
		c.ActionCache = &cache
		if c.ActionOfflineMode {
			c.ActionCache = &runner.GoGitActionCacheOfflineMode{Parent: cache}
		}
		if len(o.localRepositories) > 0 {
			local := make(map[string]string)
			for _, repository := range o.localRepositories {
				name, path, ok := strings.Cut(repository, "=")
				if !ok || name == "" || path == "" {
					return nil, errors.New("local-repository must have the form repository=path")
				}
				local[name] = resolvePath(c.Workdir, path)
			}
			c.ActionCache = &runner.LocalRepositoryCache{Parent: c.ActionCache, LocalRepositories: local, CacheDirCache: make(map[string]string)}
		}
	}
	return o, nil
}

func resolvePath(dir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

func (o *options) loadValues() error {
	c := &o.config
	for i, values := range []struct {
		target *map[string]string
		pairs  []string
		file   string
		kind   string
	}{
		{&c.Env, o.env, o.envFile, "env"},
		{&c.Inputs, o.inputs, o.inputFile, "input"},
		{&c.Secrets, o.secrets, o.secretFile, "secret"},
		{&c.Vars, o.vars, o.varFile, "var"},
	} {
		m, err := readValues(resolvePath(c.Workdir, values.file), values.pairs, values.kind, o.filesRequired[i])
		if err != nil {
			return err
		}
		*values.target = m
	}
	c.Token = c.Secrets["GITHUB_TOKEN"]
	return nil
}

// Missing default files are optional; explicit files and malformed contents are errors.
func readValues(path string, pairs []string, kind string, required bool) (map[string]string, error) {
	values := make(map[string]string)
	for _, pair := range pairs {
		key, value, hasValue := strings.Cut(pair, "=")
		if kind == "secret" || kind == "var" {
			key = strings.ToUpper(key)
		}
		if key == "" {
			return nil, errors.New("environment, secret, variable, and input names must not be empty")
		}
		if !hasValue && (kind == "secret" || kind == "var") {
			var ok bool
			value, ok = os.LookupEnv(key)
			if !ok || value == "" {
				return nil, fmt.Errorf("provide a value for %s in settings or the environment", key)
			}
		}
		values[key] = value
	}
	if path == "" {
		return values, nil
	}
	//nolint:gosec // G304: user-selected act values files are intentionally readable.
	data, err := os.ReadFile(path)
	if !required && errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read values file %s: %w", path, err)
	}
	var fromFile map[string]string
	if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" {
		err = yaml.Unmarshal(data, &fromFile)
	} else {
		fromFile, err = godotenv.Unmarshal(string(data))
	}
	// Parser errors can include secret values; only report the file.
	if err != nil {
		return nil, fmt.Errorf("invalid values file %s", path)
	}
	for key, value := range fromFile {
		if kind == "secret" {
			key = strings.ToUpper(key)
		}
		if _, exists := values[key]; !exists {
			values[key] = value
		}
	}
	return values, nil
}
