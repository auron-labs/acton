// Package settings holds the user-configurable act invocation: the settings
// fields, their .actrc file defaults, and the act command line they build.
package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kballard/go-shellquote"
)

// Settings holds the user-configurable act invocation. Fields map to act
// flags; empty fields are omitted. LoadActrc fills them from .actrc files.
type Settings struct {
	// Dir is the project directory act runs in (its -C workdir equivalent).
	Dir string

	// Workflows are the -W workflow paths relative to Dir; empty runs all.
	Workflows []string
	// Event is the act positional argument, e.g. "push".
	Event string
	// Job is -j; "" runs all jobs.
	Job string

	// Platforms are -P values, "runner=image".
	Platforms []string
	// Arch is --container-architecture.
	Arch string

	// Secrets are -s "KEY=VALUE".
	Secrets    []string
	SecretFile string
	// Env are --env "KEY=VALUE".
	Env     []string
	EnvFile string
	// Vars are --var "KEY=VALUE".
	Vars    []string
	VarFile string
	// Inputs are --input "KEY=VALUE" action inputs.
	Inputs []string
	// EventPayload is -e, a JSON file with the event payload.
	EventPayload string

	DryRun  bool
	Verbose bool
	Pull    bool
	Reuse   bool
	Bind    bool
	Quiet   bool

	// ExtraArgs holds act flags not covered above, as typed for the CLI.
	ExtraArgs string

	// ActrcFiles lists the config files defaults were loaded from.
	ActrcFiles []string

	// platformsDefaulted is true while Platforms still holds the built-in
	// default; the first -P from an .actrc replaces it instead of appending.
	platformsDefaulted bool
	// Preserve explicitly empty .actrc paths, which disable automatic file loading.
	secretFileSet, envFileSet, varFileSet bool
}

// Default returns settings used when no .actrc provides a value.
// The platform default avoids act's interactive first-run image prompt.
func Default(dir string) *Settings {
	return &Settings{
		Dir:                dir,
		Event:              "push",
		Pull:               true,
		Platforms:          []string{"ubuntu-latest=catthehacker/ubuntu:act-latest"},
		platformsDefaulted: true,
	}
}

// actrcCandidates returns the config files act itself reads, in the order it
// merges them: user config first, then the project .actrc.
func actrcCandidates(dir string) []string {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".actrc"))
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "act", "actrc"))
	}
	if dir != "" {
		paths = append(paths, filepath.Join(dir, ".actrc"))
	}
	return paths
}

// applyActrcWord maps an .actrc flag taking a value onto a settings field.
// Unlisted flags are preserved verbatim in ExtraArgs.
func (s *Settings) applyActrcWord(name, value string, hasValue bool) (ok bool) {
	switch name {
	case "-P", "--platform":
		if s.platformsDefaulted {
			s.Platforms, s.platformsDefaulted = nil, false
		}
		s.Platforms = appendUnique(s.Platforms, value)
	case "--container-architecture", "--container-arch":
		s.Arch = value
	case "-s", "--secret":
		s.Secrets = appendUnique(s.Secrets, value)
	case "--secret-file":
		s.SecretFile, s.secretFileSet = value, true
	case "--env":
		s.Env = appendUnique(s.Env, value)
	case "--env-file":
		s.EnvFile, s.envFileSet = value, true
	case "--var":
		s.Vars = appendUnique(s.Vars, value)
	case "--var-file":
		s.VarFile, s.varFileSet = value, true
	case "--input":
		s.Inputs = appendUnique(s.Inputs, value)
	case "-e", "--eventpath":
		s.EventPayload = value
	case "-j", "--job":
		s.Job = value
	case "-W", "--workflows":
		s.Workflows = appendUnique(s.Workflows, value)
	case "-n", "--dryrun":
		s.DryRun = boolFlag(value, hasValue)
	case "-v", "--verbose":
		s.Verbose = boolFlag(value, hasValue)
	case "-p", "--pull":
		s.Pull = boolFlag(value, hasValue)
	case "-r", "--reuse":
		s.Reuse = boolFlag(value, hasValue)
	case "-b", "--bind":
		s.Bind = boolFlag(value, hasValue)
	case "-q", "--quiet":
		s.Quiet = boolFlag(value, hasValue)
	default:
		return false
	}
	return true
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func boolFlag(v string, hasValue bool) bool {
	return !hasValue || v == "true" || v == "1"
}

// flagTakesValue reports whether an .actrc flag name normally carries a
// value, so a bare "--flag value" pair can be split.
func flagTakesValue(name string) bool {
	switch name {
	case "-P", "--platform", "--container-architecture", "--container-arch",
		"-s", "--secret", "--secret-file", "--env", "--env-file", "--var",
		"--var-file", "--input", "-e", "--eventpath", "-j", "--job",
		"-W", "--workflows", "--matrix", "--userns", "--container-options",
		"--github-instance", "--artifact-server-path", "--cache-server-path",
		"--remote-name", "--action-offline-path", "--network", "--workdir",
		"-C", "--directory", "--actor", "-a", "--github-token", "--input-file",
		"--defaultbranch", "--container-daemon-socket", "--container-cap-add",
		"--container-cap-drop", "--replace-ghe-action-with-github-com",
		"--replace-ghe-action-token-with-github-com", "--concurrent-jobs",
		"--action-cache-path", "--local-repository", "--cache-server-addr",
		"--cache-server-external-url", "--cache-server-port", "--artifact-server-addr",
		"--artifact-server-port":
		return true
	}
	return false
}

// LoadActrc reads the .actrc files act would consult and merges their flags
// into the settings. Files loaded are recorded in ActrcFiles; flags without
// a matching field are appended to ExtraArgs so nothing is lost.
func (s *Settings) LoadActrc() {
	s.ActrcFiles = nil
	for _, path := range actrcCandidates(s.Dir) {
		//nolint:gosec // G304: paths are the fixed .actrc locations act reads.
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Saved project snapshots replace inherited defaults, including removed values.
		saved := strings.HasPrefix(string(data), savedActrcHeader)
		eventPrefix := savedEventPrefix
		// Read snapshots from before the rename so events and cleared defaults survive.
		if strings.HasPrefix(string(data), "# Saved by actgui: complete project settings (replaces inherited GUI defaults).\n") {
			saved = true
			eventPrefix = "# actgui-event: "
		}
		if saved {
			*s = Settings{Dir: s.Dir}
		}
		loaded := false
		for _, raw := range strings.Split(string(data), "\n") {
			raw = strings.TrimSpace(raw)
			if saved && strings.HasPrefix(raw, eventPrefix) {
				s.Event = strings.TrimPrefix(raw, eventPrefix)
				continue
			}
			if raw == "" || strings.HasPrefix(raw, "#") {
				continue
			}
			loaded = true
			if saved {
				// act's .actrc uses the rest of each flag line as a literal value.
				name, value, hasValue := strings.Cut(raw, " ")
				if !hasValue {
					name, value, hasValue = strings.Cut(raw, "=")
				}
				if !s.applyActrcWord(name, value, hasValue) {
					if strings.HasPrefix(raw, name+" ") {
						s.appendExtra(name, value)
					} else {
						s.appendExtra(raw)
					}
				}
			} else {
				s.applyActrcLine(raw)
			}
		}
		if loaded {
			s.ActrcFiles = append(s.ActrcFiles, path)
		}
	}
}

// applyActrcLine applies one .actrc line (one flag, possibly with a value).
func (s *Settings) applyActrcLine(line string) {
	words, err := splitWords(line)
	if err != nil || len(words) == 0 {
		return
	}
	for i := 0; i < len(words); i++ {
		w := words[i]
		name, value, hasValue, usedNext := w, "", false, false
		if eq := strings.Index(w, "="); eq > 0 && strings.HasPrefix(w, "-") {
			name, value, hasValue = w[:eq], w[eq+1:], true
		}
		if !hasValue && flagTakesValue(name) && i+1 < len(words) {
			i++
			value, hasValue, usedNext = words[i], true, true
		}
		if s.applyActrcWord(name, value, hasValue) {
			continue
		}
		// Unknown flag: keep it verbatim so act still receives it.
		if usedNext {
			s.appendExtra(name, value)
		} else {
			s.appendExtra(w)
		}
	}
}

const savedActrcHeader = "# Saved by Acton: complete project settings (replaces inherited GUI defaults).\n"

const savedEventPrefix = "# acton-event: "

// SaveActrc atomically saves the current invocation in the project's .actrc.
// The file is private because the settings can include inline secrets.
func (s *Settings) SaveActrc() error {
	if s.Dir == "" {
		return errors.New("choose a project before saving settings")
	}
	args, err := s.Args()
	if err != nil {
		return err
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\r\n") {
			return errors.New(".actrc values cannot contain newlines; use a values file instead")
		}
	}
	f, err := os.CreateTemp(s.Dir, ".actrc-*")
	if err != nil {
		return fmt.Errorf("create .actrc: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	var contents strings.Builder
	contents.WriteString(savedActrcHeader + savedEventPrefix + args[0] + "\n")
	// The CLI accepts one flag per line and treats its value literally, without shell quotes.
	for i := 1; i < len(args)-1; i++ {
		arg := args[i]
		if flagTakesValue(arg) && i+1 < len(args)-1 {
			i++
			arg += " " + args[i]
		} else if strings.ContainsAny(arg, " \t") {
			arg = strings.Replace(arg, "=", " ", 1)
		}
		contents.WriteString(arg + "\n")
	}
	_, writeErr := f.WriteString(contents.String())
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return fmt.Errorf("write .actrc: %w", err)
	}
	path := filepath.Join(s.Dir, ".actrc")
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("save .actrc: %w", err)
	}
	if !slices.Contains(s.ActrcFiles, path) {
		s.ActrcFiles = append(s.ActrcFiles, path)
	}
	return nil
}

func (s *Settings) appendExtra(words ...string) {
	s.ExtraArgs = strings.TrimSpace(s.ExtraArgs + " " + shellquote.Join(words...))
}

// splitWords splits a shell-style flag line into words, honoring single and
// double quotes and backslash escapes.
func splitWords(s string) ([]string, error) {
	var words []string
	var b strings.Builder
	var quote rune
	esc := false
	flush := func() {
		words = append(words, b.String())
		b.Reset()
	}
	started := false
	for _, r := range s {
		switch {
		case esc:
			b.WriteRune(r)
			esc = false
		case r == '\\' && quote != '\'':
			esc = true
			started = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t':
			if started || b.Len() > 0 {
				flush()
				started = false
			}
		default:
			b.WriteRune(r)
			started = true
		}
	}
	if esc {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if started || b.Len() > 0 {
		flush()
	}
	return words, nil
}

// Args builds the act command line, including the GUI's required JSON output.
func (s *Settings) Args() ([]string, error) {
	args := []string{}
	event := strings.TrimSpace(s.Event)
	if event == "" {
		event = "push"
	}
	args = append(args, event)
	for _, w := range s.Workflows {
		args = append(args, "-W", w)
	}
	if j := strings.TrimSpace(s.Job); j != "" {
		args = append(args, "-j", j)
	}
	for _, p := range s.Platforms {
		if p = strings.TrimSpace(p); p != "" {
			args = append(args, "-P", p)
		}
	}
	if a := strings.TrimSpace(s.Arch); a != "" {
		args = append(args, "--container-architecture", a)
	}
	for _, v := range s.Secrets {
		if v = strings.TrimSpace(v); v != "" {
			args = append(args, "-s", v)
		}
	}
	if v := strings.TrimSpace(s.SecretFile); v != "" || s.secretFileSet {
		args = append(args, "--secret-file", v)
	}
	for _, v := range s.Env {
		if v = strings.TrimSpace(v); v != "" {
			args = append(args, "--env", v)
		}
	}
	if v := strings.TrimSpace(s.EnvFile); v != "" || s.envFileSet {
		args = append(args, "--env-file", v)
	}
	for _, v := range s.Vars {
		if v = strings.TrimSpace(v); v != "" {
			args = append(args, "--var", v)
		}
	}
	if v := strings.TrimSpace(s.VarFile); v != "" || s.varFileSet {
		args = append(args, "--var-file", v)
	}
	for _, v := range s.Inputs {
		if v = strings.TrimSpace(v); v != "" {
			args = append(args, "--input", v)
		}
	}
	if v := strings.TrimSpace(s.EventPayload); v != "" {
		args = append(args, "-e", v)
	}
	if s.DryRun {
		args = append(args, "-n")
	}
	if s.Verbose {
		args = append(args, "-v")
	}
	if s.Pull {
		args = append(args, "-p")
	} else {
		args = append(args, "--pull=false")
	}
	if s.Reuse {
		args = append(args, "-r")
	}
	if s.Bind {
		args = append(args, "-b")
	}
	if s.Quiet {
		args = append(args, "-q")
	}
	if extra := strings.TrimSpace(s.ExtraArgs); extra != "" {
		words, err := splitWords(extra)
		if err != nil {
			return nil, fmt.Errorf("extra args: %w", err)
		}
		args = append(args, words...)
	}
	// Override user and .actrc flags so they cannot disable the GUI protocol.
	args = append(args, "--json=true")
	return args, nil
}

// ScanWorkflows lists workflow files under .github/workflows in dir,
// returning their paths relative to dir (as act -W accepts).
func ScanWorkflows(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".github", "workflows"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			out = append(out, filepath.Join(".github", "workflows", name))
		}
	}
	return out
}
