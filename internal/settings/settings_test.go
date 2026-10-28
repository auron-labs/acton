package settings

import (
	"os"
	"path/filepath"
	"reflect"
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
	want := `push -W .github/workflows/ci.yml -j test -P ubuntu-latest=catthehacker/ubuntu:act-latest -s A=1 --env B=2 --input name=x -n -v --json=true`
	if got != want {
		t.Fatalf("args\n got: %s\nwant: %s", got, want)
	}
}

func TestArgsRequiresJSON(t *testing.T) {
	for _, tt := range []struct {
		name  string
		extra string
		want  []string
	}{
		{
			name: "zero settings",
			want: []string{"push", "--json=true"},
		},
		{
			name:  "extra args cannot disable JSON",
			extra: `--json=false --env "MESSAGE=hello world"`,
			want:  []string{"push", "--json=false", "--env", "MESSAGE=hello world", "--json=true"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Settings{ExtraArgs: tt.extra}
			got, err := s.Args()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("args\n got: %q\nwant: %q", got, tt.want)
			}
			if s.ExtraArgs != tt.extra {
				t.Fatalf("ExtraArgs changed: got %q, want %q", s.ExtraArgs, tt.extra)
			}
		})
	}
}

func TestArgsActrcCannotDisableJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".actrc"), []byte("--json=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Settings{Dir: dir}
	s.LoadActrc()
	if s.ExtraArgs != "--json=false" {
		t.Fatalf("extra=%q", s.ExtraArgs)
	}
	got, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"push", "--json=false", "--json=true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args\n got: %q\nwant: %q", got, want)
	}
}

func TestArgsInvalidExtraArgs(t *testing.T) {
	s := &Settings{ExtraArgs: `--env "unclosed`}
	args, err := s.Args()
	if err == nil || err.Error() != "extra args: unclosed quote" {
		t.Fatalf("error=%v, want extra args: unclosed quote", err)
	}
	if args != nil {
		t.Fatalf("args=%q, want nil on error", args)
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
