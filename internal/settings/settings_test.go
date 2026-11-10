package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestActrcParsing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
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
	s.Workflows = []string{".github/workflows/ci.yml", ".github/workflows/lint.yml"}
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
	want := `push -W .github/workflows/ci.yml -W .github/workflows/lint.yml -j test -P ubuntu-latest=catthehacker/ubuntu:act-latest -s A=1 --env B=2 --input name=x -n -v -p --json=true`
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
			want: []string{"push", "--pull=false", "--json=true"},
		},
		{
			name:  "extra args cannot disable JSON",
			extra: `--json=false --env "MESSAGE=hello world"`,
			want:  []string{"push", "--pull=false", "--json=false", "--env", "MESSAGE=hello world", "--json=true"},
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
	want := []string{"push", "--pull=false", "--json=false", "--json=true"}
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

func TestActrcExtraArgsPreserveQuoting(t *testing.T) {
	s := Default(t.TempDir())
	s.applyActrcLine(`--container-options "--cpus 2"`)
	args, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	if args[len(args)-3] != "--container-options" || args[len(args)-2] != "--cpus 2" {
		t.Fatalf("quoted arguments lost: %q", args)
	}
}

func TestActrcExplicitFalseAndEmpty(t *testing.T) {
	s := Default(t.TempDir())
	s.applyActrcLine(`--pull=false`)
	s.applyActrcLine(`--secret-file ""`)
	s.applyActrcLine(`--env-file ""`)
	s.applyActrcLine(`--var-file ""`)
	args, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"push", "-P", "ubuntu-latest=catthehacker/ubuntu:act-latest", "--secret-file", "", "--env-file", "", "--var-file", "", "--pull=false", "--json=true"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("explicit .actrc values lost: %q", args)
	}
}

func TestSaveActrcRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".actrc"), []byte("--env OLD=removed\n--reuse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Default(t.TempDir())
	s.Event = "workflow_dispatch"
	s.Workflows = []string{"workflow with spaces.yml"}
	s.Platforms = nil
	s.Secrets = []string{"TOKEN=quote's $literal value"}
	s.Env = []string{"MESSAGE=hello world"}
	s.ExtraArgs = `--container-options "--cpus 2" --matrix os:linux`
	s.Pull = false
	s.applyActrcLine(`--secret-file ""`)
	if err := s.SaveActrc(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.Dir, ".actrc"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions: %v, %v", info, err)
	}
	loaded := Default(s.Dir)
	loaded.LoadActrc()
	want, err := s.Args()
	if err != nil {
		t.Fatal(err)
	}
	got, err := loaded.Args()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %q, want %q, err=%v", got, want, err)
	}
	if err := loaded.SaveActrc(); err != nil {
		t.Fatal(err)
	}
	loadedAgain := Default(s.Dir)
	loadedAgain.LoadActrc()
	again, err := loadedAgain.Args()
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("repeated save changed arguments: %q, err=%v", again, err)
	}
}

func TestLoadLegacyActrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".actrc"), []byte("--env OLD=removed\n--reuse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	const legacy = "# Saved by actgui: complete project settings (replaces inherited GUI defaults).\n" +
		"# actgui-event: workflow_dispatch\n--env MESSAGE=hello world\n--pull=false\n"
	if err := os.WriteFile(filepath.Join(dir, ".actrc"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Default(dir)
	s.LoadActrc()
	want := []string{"workflow_dispatch", "--env", "MESSAGE=hello world", "--pull=false", "--json=true"}
	got, err := s.Args()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy settings: got %q, want %q, err=%v", got, want, err)
	}
	if err := s.SaveActrc(); err != nil {
		t.Fatal(err)
	}
	s = Default(dir)
	s.LoadActrc()
	got, err = s.Args()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("migrated settings: got %q, want %q, err=%v", got, want, err)
	}
}

func TestSaveActrcFailurePreservesFile(t *testing.T) {
	s := Default(t.TempDir())
	path := filepath.Join(s.Dir, ".actrc")
	const original = "--env KEEP=value\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`--env "unclosed`, "--env 'MULTILINE=first\nsecond'"} {
		s.ExtraArgs = extra
		if err := s.SaveActrc(); err == nil {
			t.Fatal("accepted invalid settings")
		}
		//nolint:gosec // G304: path is the test's temporary .actrc.
		data, err := os.ReadFile(path)
		if err != nil || string(data) != original {
			t.Fatal("failed save modified existing settings")
		}
	}
}

func TestSuggestImage(t *testing.T) {
	for label, want := range map[string]string{
		"ubuntu-22.04":     "catthehacker/ubuntu:act-22.04",
		"Ubuntu-24.04":     "catthehacker/ubuntu:act-24.04",
		"ubuntu-latest":    DefaultImage,
		"ubuntu-24.04-arm": DefaultImage,
		"self-hosted":      DefaultImage,
	} {
		if got := SuggestImage(label); got != want {
			t.Errorf("SuggestImage(%q) = %q, want %q", label, got, want)
		}
	}
}
