package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActrcParsing(t *testing.T) {
	dir := t.TempDir()
	rc := `# comment
-P ubuntu-latest=catthehacker/ubuntu:act-22.04
-P ubuntu-22.04=catthehacker/ubuntu:act-22.04
--container-architecture=linux/amd64
-s GITHUB_TOKEN=abc123
--env "FOO=bar baz"
--env-file .env.test
--var REGION=us
--input name=test
-j build
-n
--reuse
--unknown-flag value
`
	if err := os.WriteFile(filepath.Join(dir, ".actrc"), []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Default(dir)
	s.Platforms = nil
	s.LoadActrc()

	if len(s.ActrcFiles) != 1 {
		t.Fatalf("actrc files=%v", s.ActrcFiles)
	}
	if len(s.Platforms) != 2 || s.Platforms[0] != "ubuntu-latest=catthehacker/ubuntu:act-22.04" {
		t.Fatalf("platforms=%v", s.Platforms)
	}
	if s.Arch != "linux/amd64" || s.Job != "build" || !s.DryRun || !s.Reuse {
		t.Fatalf("settings=%+v", s)
	}
	if len(s.Secrets) != 1 || s.Secrets[0] != "GITHUB_TOKEN=abc123" {
		t.Fatalf("secrets=%v", s.Secrets)
	}
	if len(s.Env) != 1 || s.Env[0] != "FOO=bar baz" {
		t.Fatalf("env=%v", s.Env)
	}
	if s.EnvFile != ".env.test" || len(s.Vars) != 1 || len(s.Inputs) != 1 {
		t.Fatalf("files/vars/inputs=%v %v %v", s.EnvFile, s.Vars, s.Inputs)
	}
	if !strings.Contains(s.ExtraArgs, "--unknown-flag") {
		t.Fatalf("extra=%q", s.ExtraArgs)
	}
}

func TestArgs(t *testing.T) {
	s := Default("/proj")
	s.Workflow = ".github/workflows/ci.yml"
	s.Job = "test"
	s.Secrets = []string{"A=1"}
	s.Env = []string{"B=2"}
	s.Inputs = []string{"name=x"}
	s.DryRun = true
	s.Verbose = true
	args, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	want := `push -W .github/workflows/ci.yml -j test -P ubuntu-latest=catthehacker/ubuntu:act-latest -s A=1 --env B=2 --input name=x -n -v`
	if got != want {
		t.Fatalf("args\n got: %s\nwant: %s", got, want)
	}
}

func TestScanWorkflows(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ci.yml", "cd.yaml", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(wf, f), []byte("on: push"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := ScanWorkflows(dir)
	if len(got) != 2 {
		t.Fatalf("workflows=%v", got)
	}
}
