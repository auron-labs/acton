package app

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const workflowJobKey = "\x00workflow"

// workflowSource is a workflow file as it was when the run started.
type workflowSource struct {
	name  string // path relative to the project
	lines []string
	segs  [][]yamlSegment // syntax-highlighted lines
	err   string
	list  components.ListState
}

// workflowSources reads the workflow files the run executes: the selected
// ones, or every workflow triggered by the event.
func (s *Session) workflowSources() []*workflowSource {
	names := s.settings.Workflows
	byEvent := len(names) == 0
	if byEvent {
		names = s.workflows
	}
	var sources []*workflowSource
	for _, name := range names {
		if byEvent {
			if file, err := readWorkflow(s.settings.Dir, name); err == nil && !slices.Contains(file.events, s.settings.Event) {
				continue
			}
		}
		source := &workflowSource{name: name}
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.settings.Dir, path)
		}
		//nolint:gosec // G304: the project picker and workflow menu select this local file.
		data, err := os.ReadFile(path)
		if err != nil {
			source.err = "Cannot read workflow: " + err.Error()
		} else {
			text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			source.lines = strings.Split(text, "\n")
			source.segs = highlightYAML(source.lines)
		}
		sources = append(sources, source)
	}
	return sources
}

func (s *Session) workflowRow(c *ui.Context, selected bool) {
	t := c.Theme()
	row := ui.Row(c.Key("workflow")).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
	if selected {
		row.Background(t.Accent.Alpha(.18))
	} else if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Workflow").Cursor(ui.CursorPointer).OnClick(func() { s.selJob = workflowJobKey })
	row.Children(func() {
		components.Icon(c, lucide.FileCode()).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.TextMuted)
		components.Text(c, "Workflow").Grow(1).SingleLine()
	})
}

// workflowPane shows the source of the workflow files the run executes.
func (s *Session) workflowPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.FileCode()).Size(t.Space(5), t.Space(5)).TextColor(t.TextMuted)
			components.Text(c, "Workflow").Bold().SingleLine()
			if len(s.runWorkflows) > 1 {
				names := make([]string, len(s.runWorkflows))
				for i, source := range s.runWorkflows {
					names[i] = filepath.Base(source.name)
				}
				ui.Segmented(c, &s.workflowTab, names...).Label("Workflow file")
			}
		})
		ui.Divider(c)
		if len(s.runWorkflows) == 0 {
			ui.Column(c).Fill().Center().Children(func() {
				components.Text(c, "No workflow files run for this event.").TextColor(t.TextMuted)
			})
			return
		}
		source := s.runWorkflows[min(s.workflowTab, len(s.runWorkflows)-1)]
		if source.err != "" {
			components.Text(c, source.err).Padding(t.Space(3)).TextColor(t.Danger)
			return
		}
		width := t.Space(float32(len(strconv.Itoa(len(source.lines)))) + 2)
		components.List(c.Key(source.name), &source.list, len(source.lines), func(i int) {
			ui.Row(c).FillWidth().Padding(0, t.Space(3)).Gap(t.Space(3)).Children(func() {
				components.Text(c, strconv.Itoa(i+1)).Font("JetBrains Mono").FontSize(12).TextColor(t.TextMuted).TextAlign(ui.End).Width(width).Shrink(0).SingleLine()
				ui.RichText(c, yamlSpans(t, source.lines[i], source.segs[i])...).Font("JetBrains Mono").FontSize(12).Selectable()
			})
		}).Fill().Label("Workflow source")
	})
}
