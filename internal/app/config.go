package app

import (
	"acton/internal/settings"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

var workflowEvents = []string{"push", "pull_request", "workflow_dispatch", "schedule", "release", "issues", "create", "delete", "fork"}

func (s *Session) configView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(panelBackground(c)).Children(func() {
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				s.targetView(c)
				configSection(c, "Images & platforms", fmt.Sprintf("%d mappings", len(s.settings.Platforms)), lucide.Container(), &s.configOpen[0], func() { s.imagesView(c) })
				configSection(c, "Environment", s.environmentSummary(), lucide.KeyRound(), &s.configOpen[1], func() { s.environmentView(c) })
				configSection(c, "Options", fmt.Sprintf("%d on", s.optionCount()), lucide.SlidersHorizontal(), &s.configOpen[2], func() {
					for _, option := range []struct {
						value *bool
						label string
					}{
						{&s.settings.DryRun, "Dry run (plan only)"},
						{&s.settings.Verbose, "Verbose output"},
						{&s.settings.Pull, "Always pull images"},
						{&s.settings.Reuse, "Reuse containers"},
						{&s.settings.Bind, "Bind workdir (no copy)"},
						{&s.settings.Quiet, "Quiet (no progress)"},
					} {
						components.Switch(c, option.value, option.label, components.SwitchOptions{Compact: true})
					}
				})
				configSection(c, "Advanced", "Payload, extra args", lucide.Terminal(), &s.configOpen[3], func() {
					field(c, "Event payload file", func() {
						ui.Row(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
							styleConfigInput(c, components.TextBox(c, &s.settings.EventPayload, components.TextBoxOptions{}).
								Placeholder("path/to/event.json").Label("Event payload JSON file").Grow(1).MinWidth(0))
							if components.Button(c, "Browse", components.ButtonSecondary).Shrink(0).Clicked() {
								s.pickConfigFile("Event payload")
							}
						})
					})
					field(c, "Extra act args", func() {
						styleConfigInput(c, components.TextBox(c, &s.settings.ExtraArgs, components.TextBoxOptions{Multiline: true}).
							Placeholder("--matrix os:linux …").Label("Additional act flags").Lines(3, 3))
					})
					components.Text(c, "Passed verbatim to act.").FontSize(t.Rem(11.0 / 13)).TextColor(t.TextMuted)
				})
				s.app.runPreferencesView(c)
				if s.fileErr != "" {
					components.Text(c, s.fileErr).Padding(t.Space(3)).TextColor(t.Danger)
				}
			})
		})
		s.commandPreview(c)
	})
}

func configSection(c *ui.Context, title, summary string, icon *ui.SVG, open *bool, content func()) {
	t := c.Theme()
	ui.Column(c.Key(title)).FillWidth().Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		section := ui.CollapsibleBase(c, open)
		section.Trigger.FillWidth().Padding(t.Space(3.25), t.Space(4)).Gap(t.Space(2.5)).Label(title).Children(func() {
			components.Icon(c, icon).Size(t.Space(4), t.Space(4)).TextColor(t.TextMuted).Shrink(0)
			components.Text(c, title).FontWeight(500).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
			if title == "Options" {
				components.Badge(c, summary, components.BadgeOptions{Tone: components.BadgeAccent})
			} else {
				components.Text(c, summary).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).SingleLine().Shrink(0)
			}
			chevron := lucide.ChevronRight()
			if *open {
				chevron = lucide.ChevronDown()
			}
			components.Icon(c, chevron).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.TextMuted).Shrink(0)
		})
		section.Panel(content).FillWidth().Gap(t.Space(3)).Padding(0, t.Space(4), t.Space(4), t.Space(10.5))
	})
}

func (s *Session) targetView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(t.Space(4)).Gap(t.Space(3.5)).BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		field(c, "Workflows", func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				for _, w := range s.workflows {
					selected := slices.Contains(s.settings.Workflows, w)
					was := selected
					components.Checkbox(c.Key(w), &selected, filepath.Base(w))
					if selected != was {
						s.toggleWorkflow(w, selected)
					}
				}
			})
		})
		if s.workflowErr != "" {
			components.Text(c, s.workflowErr).TextColor(t.Warning).FontSize(t.Rem(12.0 / 13))
		} else if s.workflowMeta != "" {
			ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.GitBranch()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted).Shrink(0)
				components.Text(c, s.workflowMeta).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted)
			})
		}
		ui.Row(c).FillWidth().Gap(t.Space(2.5)).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				field(c, "Event", func() {
					opts := make([]components.NativeSelectOption[string], 0, len(workflowEvents)+1)
					for _, e := range workflowEvents {
						opts = append(opts, components.NativeSelectOption[string]{Value: e, Label: e})
					}
					opts = append(opts, components.NativeSelectOption[string]{Value: "custom", Label: "Custom…"})
					sel := components.NativeSelect(c, &s.eventSel, opts)
					sel.Trigger.FillWidth().Label("Event")
					if sel.Changed() {
						if s.eventSel == "custom" {
							s.eventOther = s.settings.Event
						} else {
							s.settings.Event = s.eventSel
						}
					}
				})
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				field(c, "Job", func() {
					if len(s.workflowJobs) == 0 {
						styleConfigInput(c, components.TextBox(c, &s.settings.Job, components.TextBoxOptions{}).Placeholder("All jobs").Label("Job ID filter"))
						return
					}
					opts := []components.NativeSelectOption[string]{{Value: "", Label: "All jobs"}}
					for _, job := range s.workflowJobs {
						opts = append(opts, components.NativeSelectOption[string]{Value: job, Label: job})
					}
					if s.settings.Job != "" && !slices.Contains(s.workflowJobs, s.settings.Job) {
						opts = append(opts, components.NativeSelectOption[string]{Value: s.settings.Job, Label: s.settings.Job})
					}
					components.NativeSelect(c, &s.settings.Job, opts).Trigger.FillWidth().Label("Job ID filter")
				})
			})
		})
		if s.eventSel == "custom" {
			input := components.TextBox(c, &s.eventOther, components.TextBoxOptions{}).Placeholder("event name").Label("Custom event name")
			styleConfigInput(c, input)
			if input.Changed() {
				s.settings.Event = s.eventOther
			}
		}
	})
}

func (s *Session) imagesView(c *ui.Context) {
	t := c.Theme()
	if len(s.settings.Platforms) > 0 {
		ui.Column(c).FillWidth().Radius(t.Radius).Border(1, sectionBorder(c)).Background(inputBackground(c)).Dividers(1, sectionBorder(c)).Children(func() {
			for i := range s.settings.Platforms {
				s.mappingRow(c, i)
			}
		})
	}
	if components.Button(c, "+ Add mapping", components.ButtonGhost).TextColor(t.Accent).Clicked() {
		s.settings.Platforms = append(s.settings.Platforms, "="+settings.DefaultImage)
	}
	field(c, "Container architecture", func() {
		values := []string{"linux/amd64", "linux/arm64", ""}
		selected := slices.Index(values, s.settings.Arch)
		if selected < 0 {
			styleConfigInput(c, components.TextBox(c, &s.settings.Arch, components.TextBoxOptions{}).Label("Container architecture"))
			return
		}
		segments := ui.Segmented(c, &selected, "amd64", "arm64", "Default").FillWidth().Label("Container architecture")
		if segments.Changed() {
			s.settings.Arch = values[selected]
		}
	})
}

// mappingRow is one runner=image pair inside the shared mappings group: the
// runner label on the first line, the container image behind a branch arrow
// on the second. Both stay plain editable text.
func (s *Session) mappingRow(c *ui.Context, i int) {
	t := c.Theme()
	runner, image, _ := strings.Cut(s.settings.Platforms[i], "=")
	ui.Column(c.Key(i)).FillWidth().Gap(t.Space(0.75)).Padding(t.Space(2), t.Space(2.5)).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
			input := ui.TextInputBase(c, &runner).Grow(1).MinWidth(0).Placeholder("ubuntu-latest").Label(fmt.Sprintf("Runner label %d", i+1))
			input.Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).TextColor(t.Text)
			if input.Changed() {
				s.settings.Platforms[i] = runner + "=" + image
			}
			remove := components.Button(c, "", components.ButtonGhost).Label(fmt.Sprintf("Remove mapping %d", i+1)).Size(t.Space(4), t.Space(4)).Padding(0).Shrink(0)
			remove.Children(func() { components.Icon(c, lucide.X()).Size(t.Space(2.75), t.Space(2.75)).TextColor(t.TextMuted) })
			remove.OnClick(func() { s.settings.Platforms = slices.Delete(s.settings.Platforms, i, i+1) })
		})
		ui.Row(c).FillWidth().Gap(t.Space(1.25)).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CornerDownRight()).Size(t.Space(2.75), t.Space(2.75)).TextColor(t.TextMuted.Alpha(0.7)).Shrink(0)
			input := ui.TextInputBase(c, &image).Grow(1).MinWidth(0).Placeholder("image required").Label(fmt.Sprintf("Container image %d", i+1))
			input.Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).TextColor(t.Text)
			if image == "" {
				// An unset image gets a warning outline so it cannot pass for a filled row.
				input.Border(1, t.Warning).Radius(t.Radius).Padding(t.Space(0.5), t.Space(1))
			}
			if input.Changed() {
				s.settings.Platforms[i] = runner + "=" + image
			}
		})
	})
}

func (s *Session) environmentSummary() string {
	count := len(s.settings.Env) + len(s.settings.Vars) + len(s.settings.Inputs)
	return fmt.Sprintf("%d secrets · %d vars", len(s.settings.Secrets), count)
}

func (s *Session) environmentView(c *ui.Context) {
	t := c.Theme()
	labels := []string{"Secrets", "Env", "Vars", "Inputs"}
	lists := []*[]string{&s.settings.Secrets, &s.settings.Env, &s.settings.Vars, &s.settings.Inputs}
	tabLabels := slices.Clone(labels)
	for i, list := range lists {
		if len(*list) > 0 {
			tabLabels[i] += fmt.Sprintf(" %d", len(*list))
		}
	}
	ui.Segmented(c, &s.envTab, tabLabels...).FillWidth().FontSize(t.Rem(11.0 / 13)).Label("Environment editor")
	ui.Column(c.Key(s.envTab)).FillWidth().Gap(t.Space(3)).Children(func() {
		list := lists[s.envTab]
		for i := range *list {
			row := ui.Row(c.Key(i)).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center)
			row.Children(func() {
				type secretVisibility struct {
					value string
					show  bool
				}
				visibility := ui.Local(row, "secret-visibility", func() secretVisibility { return secretVisibility{value: (*list)[i]} })
				if visibility.value != (*list)[i] {
					*visibility = secretVisibility{value: (*list)[i]}
				}
				name, value, _ := strings.Cut((*list)[i], "=")
				key := components.TextBox(c, &name, components.TextBoxOptions{}).Placeholder("NAME").Label(fmt.Sprintf("%s name %d", labels[s.envTab], i+1)).Grow(1).Basis(0).MinWidth(0)
				styleConfigInput(c, key)
				input := components.TextBox(c, &value, components.TextBoxOptions{}).Placeholder("value").Label(fmt.Sprintf("%s value %d", labels[s.envTab], i+1)).Grow(1).Basis(0).MinWidth(0)
				styleConfigInput(c, input)
				if s.envTab == 0 {
					label, icon := "Show secret", lucide.Eye()
					if visibility.show {
						label, icon = "Hide secret", lucide.EyeOff()
					}
					show := components.Button(c, "", components.ButtonGhost).Label(fmt.Sprintf("%s %d", label, i+1)).Size(t.Space(5), t.Space(7)).Padding(0).Shrink(0)
					show.Children(func() { components.Icon(c, icon).Size(t.Space(3), t.Space(3)).TextColor(t.TextMuted) })
					if show.Clicked() {
						visibility.show = !visibility.show
					}
					if !visibility.show {
						input.Password()
					}
				}
				if key.Changed() || input.Changed() {
					(*list)[i] = name + "=" + value
					visibility.value = (*list)[i]
				}
				remove := components.Button(c, "", components.ButtonGhost).Label(fmt.Sprintf("Remove %s %d", labels[s.envTab], i+1)).Size(t.Space(5), t.Space(7)).Padding(0).Shrink(0)
				remove.Children(func() { components.Icon(c, lucide.X()).Size(t.Space(3), t.Space(3)) })
				remove.OnClick(func() { *list = slices.Delete(*list, i, i+1) })
			})
		}
		ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
			addLabel := []string{"Add secret", "Add env", "Add var", "Add input"}[s.envTab]
			if components.Button(c, "+ "+addLabel, components.ButtonGhost).TextColor(t.Accent).Clicked() {
				*list = append(*list, "=")
			}
			if s.envTab < 3 && components.Button(c, "Import .env", components.ButtonGhost).TextColor(t.Accent).Clicked() {
				s.pickConfigFile(labels[s.envTab])
			}
		})
		if s.envTab < 3 {
			file := []*string{&s.settings.SecretFile, &s.settings.EnvFile, &s.settings.VarFile}[s.envTab]
			field(c, "Loaded file", func() {
				styleConfigInput(c, components.TextBox(c, file, components.TextBoxOptions{}).Placeholder("No file selected").Label(labels[s.envTab]+" file"))
			})
		}
	})
}

func (s *Session) optionCount() int {
	n := 0
	for _, on := range []bool{s.settings.DryRun, s.settings.Verbose, s.settings.Pull, s.settings.Reuse, s.settings.Bind, s.settings.Quiet} {
		if on {
			n++
		}
	}
	return n
}

func (s *Session) commandPreview(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(4)).Gap(t.Space(3)).BorderWidth(1, 0, 0, 0).BorderColor(sectionBorder(c)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			components.Text(c, "Command preview").FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).Grow(1)
			if components.Button(c, "Save", components.ButtonSecondary).Label("Save settings to .actrc").Disabled(s.settings.Dir == "").Clicked() {
				s.fileErr = ""
				if err := s.settings.SaveActrc(); err != nil {
					s.fileErr = err.Error()
				}
			}
			copyButton := components.Button(c, "", components.ButtonGhost).Label("Copy command").Size(t.Space(6), t.Space(5)).Padding(0)
			copyButton.Children(func() { components.Icon(c, lucide.Copy()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted) })
			if copyButton.Clicked() {
				c.WriteClipboard(s.cmdline())
			}
		})
		ui.Themed(c, logTheme(c)).FillWidth().Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius).Border(1, sectionBorder(c)).Children(func() {
			components.Text(c, s.cmdline()).Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).MaxLines(3).Ellipsis("…").Selectable()
		})
		if _, err := s.settings.Args(); err != nil {
			components.Text(c, err.Error()).FontSize(t.Rem(11.0 / 13)).TextColor(t.Danger)
		}
		if len(s.settings.ActrcFiles) > 0 {
			ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.FileText()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted)
				components.Text(c, "Defaults from .actrc").FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).Grow(1)
				if components.Button(c, "View", components.ButtonGhost).TextColor(t.Accent).Padding(0).Clicked() {
					s.showDefaults = !s.showDefaults
				}
			})
			if s.showDefaults {
				for _, file := range s.settings.ActrcFiles {
					components.Text(c, abbrevHome(file)).FontSize(t.Rem(11.0 / 13)).TextColor(t.TextMuted)
				}
			}
		}
	})
}

// toggleWorkflow adds or removes a workflow from the selection, keeping the
// project's file order.
func (s *Session) toggleWorkflow(workflow string, selected bool) {
	var next []string
	for _, name := range s.workflows {
		if name == workflow && selected || name != workflow && slices.Contains(s.settings.Workflows, name) {
			next = append(next, name)
		}
	}
	s.settings.Workflows = next
	s.loadWorkflow()
}
