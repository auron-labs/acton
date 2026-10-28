package act

import (
	"acton/internal/settings"
	"os"
	"path/filepath"
	"testing"
)

func TestOptionsSettings(t *testing.T) {
	dir := t.TempDir()
	s := settings.Default(dir)
	s.Workflow = "ci.yml"
	s.Job = "build"
	s.DryRun, s.Reuse, s.Bind, s.Quiet, s.Verbose = true, true, true, true, true
	s.Env = []string{"MESSAGE=hello world"}
	s.Secrets = []string{"token=override"}
	s.Vars = []string{"region=eu"}
	s.Inputs = []string{"name=test"}
	s.EventPayload = "event.json"
	s.ExtraArgs = `--matrix os:linux --matrix os:macos --container-options "--cpus 2" --pull=false --json=false`
	args, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseOptions(dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.loadValues(); err != nil {
		t.Fatal(err)
	}
	c := &o.config
	if c.Workdir != dir || o.workflows != filepath.Join(dir, "ci.yml") || c.EventPath != filepath.Join(dir, "event.json") || o.job != "build" {
		t.Fatalf("paths/filter: %+v", o)
	}
	if !o.dryrun || !o.verbose || !c.ReuseContainers || !c.BindWorkdir || c.LogOutput || c.ForcePull || !c.JSONLogger {
		t.Fatal("boolean settings lost")
	}
	if c.Env["MESSAGE"] != "hello world" || c.Secrets["TOKEN"] != "override" || c.Vars["REGION"] != "eu" || c.Inputs["name"] != "test" {
		t.Fatal("values not mapped")
	}
	if !c.Matrix["os"]["linux"] || !c.Matrix["os"]["macos"] || c.ContainerOptions != "--cpus 2" {
		t.Fatal("extra execution flags not mapped")
	}
}

func TestOptionsFiles(t *testing.T) {
	dir := t.TempDir()
	//nolint:gosec // G101: synthetic credentials exercise file loading.
	for name, content := range map[string]string{
		".env":         "MESSAGE=from-file\nFILE_ONLY=loaded\n",
		".input":       "name=from-file\n",
		".vars":        "region=us\n",
		".secrets":     "github_token=file-token\n",
		"secrets.yaml": "sample: yaml-secret\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FROM_ENV", "environment-secret")
	o, err := parseOptions(dir, []string{"push", "--env", "MESSAGE=override", "-s", "FROM_ENV", "--input", "name=explicit"})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.loadValues(); err != nil {
		t.Fatal(err)
	}
	c := &o.config
	if c.Env["MESSAGE"] != "override" || c.Env["FILE_ONLY"] != "loaded" || c.Inputs["name"] != "explicit" || c.Token != "file-token" || c.Secrets["FROM_ENV"] != "environment-secret" || c.Vars["region"] != "us" {
		t.Fatal("file defaults or precedence lost")
	}
	o, err = parseOptions(dir, []string{"--secret-file", "secrets.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.loadValues(); err != nil {
		t.Fatal(err)
	}
	if o.config.Secrets["SAMPLE"] != "yaml-secret" {
		t.Fatal("YAML secrets not loaded")
	}
}

func TestOptionsInvalid(t *testing.T) {
	for _, args := range [][]string{
		{"--matrix", "invalid"},
		{"-P", "invalid"},
		{"--local-repository", "invalid"},
		{"--concurrent-jobs=-1"},
		{"push", "pull_request"},
		{"--watch"},
		{"--unknown"},
	} {
		if _, err := parseOptions(t.TempDir(), args); err == nil {
			t.Fatalf("accepted invalid args %q", args)
		}
	}
}

func TestOptionsFileErrors(t *testing.T) {
	for _, args := range [][]string{{"--secret-file", "missing"}, {"-s", "ACTON_UNSET_SECRET"}} {
		o, err := parseOptions(t.TempDir(), args)
		if err != nil {
			t.Fatal(err)
		}
		if err := o.loadValues(); err == nil {
			t.Fatalf("accepted missing values for %q", args)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".secrets"), []byte("bad-key=private-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := parseOptions(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.loadValues(); err == nil || err.Error() != "invalid values file "+filepath.Join(dir, ".secrets") {
		t.Fatalf("unsafe or missing parse error: %v", err)
	}
}

func TestOptionsDisabledDefaultFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".env", ".secrets", ".vars"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("UNWANTED=value\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	o, err := parseOptions(dir, []string{"--env-file=", "--secret-file=", "--var-file=", "--pull=false"})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.loadValues(); err != nil {
		t.Fatal(err)
	}
	if len(o.config.Env)+len(o.config.Secrets)+len(o.config.Vars) != 0 || o.config.ForcePull {
		t.Fatal("disabled defaults were enabled")
	}
}
