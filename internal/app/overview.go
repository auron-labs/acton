package app

import (
	"acton/internal/act"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const overviewJobKey = "\x00overview"

func (a *App) overviewRow(c *ui.Context, selected bool) {
	t := c.Theme()
	row := ui.Row(c.Key("overview")).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
	if selected {
		row.Background(t.Accent.Alpha(.18))
	} else if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Overview").Cursor(ui.CursorPointer).OnClick(func() { a.selJob = overviewJobKey })
	row.Children(func() {
		components.Icon(c, lucide.List()).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.TextMuted)
		components.Text(c, "Overview").Grow(1).SingleLine()
	})
}

func (a *App) overviewPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		components.Text(c, "Overview").Bold().Padding(t.Space(3))
		ui.Divider(c)
		count := 0
		for _, j := range a.run.Jobs {
			count += len(j.Annotations)
		}
		if count == 0 {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				components.Text(c, "No annotations").Bold()
				components.Text(c, "GitHub Actions errors, warnings and notices will appear here.").TextColor(t.TextMuted)
			})
			return
		}
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
				for _, j := range a.run.Jobs {
					if len(j.Annotations) == 0 {
						continue
					}
					ui.Column(c.Key(j.Key)).FillWidth().Gap(6).Children(func() { components.Text(c, j.Name).Bold(); annotationList(c, j.Annotations) })
				}
			})
		})
	})
}

func annotationList(c *ui.Context, annotations []act.Annotation) {
	t := c.Theme()
	files := make([]string, 0)
	grouped := map[string][]act.Annotation{}
	for _, a := range annotations {
		file := a.File
		if file == "" {
			file = "General"
		}
		if _, ok := grouped[file]; !ok {
			files = append(files, file)
		}
		grouped[file] = append(grouped[file], a)
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		for _, file := range files {
			ui.Column(c.Key(file)).FillWidth().Padding(8, 12).Gap(5).Children(func() {
				components.Text(c, fmt.Sprintf("%s  (%d)", file, len(grouped[file]))).Font("JetBrains Mono").FontSize(11).TextColor(t.TextMuted)
				for _, a := range grouped[file] {
					color := t.Accent
					switch a.Severity {
					case "error":
						color = t.Danger
					case "warning":
						color = t.Warning
					}
					location := a.Line
					if a.Column != "" {
						location += ":" + a.Column
					}
					text := a.Message
					if a.Title != "" {
						text = a.Title + ": " + text
					}
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						components.Text(c, strings.ToUpper(a.Severity)).FontSize(10).TextColor(color).Shrink(0)
						components.Text(c, text).Grow(1).MinWidth(0).Selectable()
						if location != "" {
							components.Text(c, location).FontSize(11).TextColor(t.TextMuted).Shrink(0)
						}
					})
				}
			})
		}
	})
}
