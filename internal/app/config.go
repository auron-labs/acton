package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

var workflowEvents = []string{"push", "pull_request", "workflow_dispatch", "schedule", "release", "issues", "create", "delete", "fork"}

func (a *App) configView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(panelBackground(c)).Children(func() {
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				a.targetView(c)
				configSection(c, "Images & platforms", fmt.Sprintf("%d mappings", len(a.settings.Platforms)), lucide.Container(), &a.configOpen[0], func() { a.imagesView(c) })
				configSection(c, "Environment", a.environmentSummary(), lucide.KeyRound(), &a.configOpen[1], func() { a.environmentView(c) })
				configSection(c, "Options", fmt.Sprintf("%d on", a.optionCount()), lucide.SlidersHorizontal(), &a.configOpen[2], func() {
					for _, option := range []struct {
						value *bool
						label string
					}{
						{&a.settings.DryRun, "Dry run (plan only)"},
						{&a.settings.Verbose, "Verbose output"},
						{&a.settings.Pull, "Always pull images"},
						{&a.settings.Reuse, "Reuse containers"},
						{&a.settings.Bind, "Bind workdir (no copy)"},
						{&a.settings.Quiet, "Quiet (no progress)"},
					} {
						components.Switch(c, option.value, option.label, components.SwitchOptions{Compact: true})
					}
				})
				configSection(c, "Advanced", "Payload, extra args", lucide.Terminal(), &a.configOpen[3], func() {
					field(c, "Event payload file", func() {
						ui.Row(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
							styleConfigInput(c, components.TextBox(c, &a.settings.EventPayload, components.TextBoxOptions{}).
								Placeholder("path/to/event.json").Label("Event payload JSON file").Grow(1).MinWidth(0))
							if components.Button(c, "Browse", components.ButtonSecondary).Shrink(0).Clicked() {
								a.pickConfigFile("Event payload")
							}
						})
					})
					field(c, "Extra act args", func() {
						styleConfigInput(c, components.TextBox(c, &a.settings.ExtraArgs, components.TextBoxOptions{Multiline: true}).
							Placeholder("--matrix os:linux …").Label("Additional act flags").Lines(3, 3))
					})
					components.Text(c, "Passed verbatim to act.").FontSize(t.Rem(11.0 / 13)).TextColor(t.TextMuted)
				})
				a.runPreferencesView(c)
				if a.fileErr != "" {
					components.Text(c, a.fileErr).Padding(t.Space(3)).TextColor(t.Danger)
				}
			})
		})
		a.commandPreview(c)
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

func (a *App) targetView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(t.Space(4)).Gap(t.Space(3.5)).BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		field(c, "Workflow", func() {
			opts := make([]components.NativeSelectOption[string], 0, len(a.workflows)+1)
			opts = append(opts, components.NativeSelectOption[string]{Value: "", Label: "All workflows"})
			for _, w := range a.workflows {
				opts = append(opts, components.NativeSelectOption[string]{Value: w, Label: filepath.Base(w)})
			}
			sel := components.NativeSelect(c, &a.settings.Workflow, opts)
			sel.Trigger.FillWidth().Label("Workflow").Font("JetBrains Mono").FontSize(t.Rem(12.0 / 13))
			if sel.Changed() {
				a.loadWorkflow()
			}
		})
		if a.workflowErr != "" {
			components.Text(c, a.workflowErr).TextColor(t.Warning).FontSize(t.Rem(12.0 / 13))
		} else if a.workflowMeta != "" {
			ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.GitBranch()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted).Shrink(0)
				components.Text(c, a.workflowMeta).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted)
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
					sel := components.NativeSelect(c, &a.eventSel, opts)
					sel.Trigger.FillWidth().Label("Event")
					if sel.Changed() {
						if a.eventSel == "custom" {
							a.eventOther = a.settings.Event
						} else {
							a.settings.Event = a.eventSel
						}
					}
				})
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				field(c, "Job", func() {
					if len(a.workflowJobs) == 0 {
						styleConfigInput(c, components.TextBox(c, &a.settings.Job, components.TextBoxOptions{}).Placeholder("All jobs").Label("Job ID filter"))
						return
					}
					opts := []components.NativeSelectOption[string]{{Value: "", Label: "All jobs"}}
					for _, job := range a.workflowJobs {
						opts = append(opts, components.NativeSelectOption[string]{Value: job, Label: job})
					}
					if a.settings.Job != "" && !slices.Contains(a.workflowJobs, a.settings.Job) {
						opts = append(opts, components.NativeSelectOption[string]{Value: a.settings.Job, Label: a.settings.Job})
					}
					components.NativeSelect(c, &a.settings.Job, opts).Trigger.FillWidth().Label("Job ID filter")
				})
			})
		})
		if a.eventSel == "custom" {
			input := components.TextBox(c, &a.eventOther, components.TextBoxOptions{}).Placeholder("event name").Label("Custom event name")
			styleConfigInput(c, input)
			if input.Changed() {
				a.settings.Event = a.eventOther
			}
		}
	})
}

func (a *App) imagesView(c *ui.Context) {
	t := c.Theme()
	if len(a.settings.Platforms) > 0 {
		ui.Column(c).FillWidth().Radius(t.Radius).Border(1, sectionBorder(c)).Background(inputBackground(c)).Dividers(1, sectionBorder(c)).Children(func() {
			for i := range a.settings.Platforms {
				a.mappingRow(c, i)
			}
		})
	}
	if components.Button(c, "+ Add mapping", components.ButtonGhost).TextColor(t.Accent).Clicked() {
		a.settings.Platforms = append(a.settings.Platforms, "=")
	}
	field(c, "Container architecture", func() {
		values := []string{"linux/amd64", "linux/arm64", ""}
		selected := slices.Index(values, a.settings.Arch)
		if selected < 0 {
			styleConfigInput(c, components.TextBox(c, &a.settings.Arch, components.TextBoxOptions{}).Label("Container architecture"))
			return
		}
		segments := ui.Segmented(c, &selected, "amd64", "arm64", "Default").FillWidth().Label("Container architecture")
		if segments.Changed() {
			a.settings.Arch = values[selected]
		}
	})
}

// mappingRow is one runner=image pair inside the shared mappings group: the
// runner label on the first line, the container image behind a branch arrow
// on the second. Both stay plain editable text.
func (a *App) mappingRow(c *ui.Context, i int) {
	t := c.Theme()
	runner, image, _ := strings.Cut(a.settings.Platforms[i], "=")
	ui.Column(c.Key(i)).FillWidth().Gap(t.Space(0.75)).Padding(t.Space(2), t.Space(2.5)).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
			input := ui.TextInputBase(c, &runner).Grow(1).MinWidth(0).Placeholder("ubuntu-latest").Label(fmt.Sprintf("Runner label %d", i+1))
			input.Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).TextColor(t.Text)
			if input.Changed() {
				a.settings.Platforms[i] = runner + "=" + image
			}
			remove := components.Button(c, "", components.ButtonGhost).Label(fmt.Sprintf("Remove mapping %d", i+1)).Size(t.Space(4), t.Space(4)).Padding(0).Shrink(0)
			remove.Children(func() { components.Icon(c, lucide.X()).Size(t.Space(2.75), t.Space(2.75)).TextColor(t.TextMuted) })
			remove.OnClick(func() { a.settings.Platforms = slices.Delete(a.settings.Platforms, i, i+1) })
		})
		ui.Row(c).FillWidth().Gap(t.Space(1.25)).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CornerDownRight()).Size(t.Space(2.75), t.Space(2.75)).TextColor(t.TextMuted.Alpha(0.7)).Shrink(0)
			input := ui.TextInputBase(c, &image).Grow(1).MinWidth(0).Placeholder("catthehacker/ubuntu:act-latest").Label(fmt.Sprintf("Container image %d", i+1))
			input.Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).TextColor(t.TextMuted)
			if input.Changed() {
				a.settings.Platforms[i] = runner + "=" + image
			}
		})
	})
}

func (a *App) environmentSummary() string {
	count := len(a.settings.Env) + len(a.settings.Vars) + len(a.settings.Inputs)
	return fmt.Sprintf("%d secrets · %d vars", len(a.settings.Secrets), count)
}

func (a *App) environmentView(c *ui.Context) {
	t := c.Theme()
	labels := []string{"Secrets", "Env", "Vars", "Inputs"}
	lists := []*[]string{&a.settings.Secrets, &a.settings.Env, &a.settings.Vars, &a.settings.Inputs}
	tabLabels := slices.Clone(labels)
	for i, list := range lists {
		if len(*list) > 0 {
			tabLabels[i] += fmt.Sprintf(" %d", len(*list))
		}
	}
	ui.Segmented(c, &a.envTab, tabLabels...).FillWidth().FontSize(t.Rem(11.0 / 13)).Label("Environment editor")
	ui.Column(c.Key(a.envTab)).FillWidth().Gap(t.Space(3)).Children(func() {
		list := lists[a.envTab]
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
				key := components.TextBox(c, &name, components.TextBoxOptions{}).Placeholder("NAME").Label(fmt.Sprintf("%s name %d", labels[a.envTab], i+1)).Grow(1).Basis(0).MinWidth(0)
				styleConfigInput(c, key)
				input := components.TextBox(c, &value, components.TextBoxOptions{}).Placeholder("value").Label(fmt.Sprintf("%s value %d", labels[a.envTab], i+1)).Grow(1).Basis(0).MinWidth(0)
				styleConfigInput(c, input)
				if a.envTab == 0 {
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
				remove := components.Button(c, "", components.ButtonGhost).Label(fmt.Sprintf("Remove %s %d", labels[a.envTab], i+1)).Size(t.Space(5), t.Space(7)).Padding(0).Shrink(0)
				remove.Children(func() { components.Icon(c, lucide.X()).Size(t.Space(3), t.Space(3)) })
				remove.OnClick(func() { *list = slices.Delete(*list, i, i+1) })
			})
		}
		ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
			addLabel := []string{"Add secret", "Add env", "Add var", "Add input"}[a.envTab]
			if components.Button(c, "+ "+addLabel, components.ButtonGhost).TextColor(t.Accent).Clicked() {
				*list = append(*list, "=")
			}
			if a.envTab < 3 && components.Button(c, "Import .env", components.ButtonGhost).TextColor(t.Accent).Clicked() {
				a.pickConfigFile(labels[a.envTab])
			}
		})
		if a.envTab < 3 {
			file := []*string{&a.settings.SecretFile, &a.settings.EnvFile, &a.settings.VarFile}[a.envTab]
			field(c, "Loaded file", func() {
				styleConfigInput(c, components.TextBox(c, file, components.TextBoxOptions{}).Placeholder("No file selected").Label(labels[a.envTab]+" file"))
			})
		}
	})
}

func (a *App) optionCount() int {
	n := 0
	for _, on := range []bool{a.settings.DryRun, a.settings.Verbose, a.settings.Pull, a.settings.Reuse, a.settings.Bind, a.settings.Quiet} {
		if on {
			n++
		}
	}
	return n
}

func (a *App) commandPreview(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(4)).Gap(t.Space(3)).BorderWidth(1, 0, 0, 0).BorderColor(sectionBorder(c)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			components.Text(c, "Command preview").FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).Grow(1)
			if components.Button(c, "Save", components.ButtonSecondary).Label("Save settings to .actrc").Disabled(a.settings.Dir == "").Clicked() {
				a.fileErr = ""
				if err := a.settings.SaveActrc(); err != nil {
					a.fileErr = err.Error()
				}
			}
			copyButton := components.Button(c, "", components.ButtonGhost).Label("Copy command").Size(t.Space(6), t.Space(5)).Padding(0)
			copyButton.Children(func() { components.Icon(c, lucide.Copy()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted) })
			if copyButton.Clicked() {
				c.WriteClipboard(a.cmdline())
			}
		})
		ui.Themed(c, logTheme(c)).FillWidth().Padding(t.Space(2.5), t.Space(3)).Radius(t.Radius).Border(1, sectionBorder(c)).Children(func() {
			components.Text(c, a.cmdline()).Font("JetBrains Mono").FontSize(t.Rem(11.0 / 13)).MaxLines(3).Ellipsis("…").Selectable()
		})
		if _, err := a.settings.Args(); err != nil {
			components.Text(c, err.Error()).FontSize(t.Rem(11.0 / 13)).TextColor(t.Danger)
		}
		if len(a.settings.ActrcFiles) > 0 {
			ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.FileText()).Size(t.Space(3.25), t.Space(3.25)).TextColor(t.TextMuted)
				components.Text(c, "Defaults from .actrc").FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).Grow(1)
				if components.Button(c, "View", components.ButtonGhost).TextColor(t.Accent).Padding(0).Clicked() {
					a.showDefaults = !a.showDefaults
				}
			})
			if a.showDefaults {
				for _, file := range a.settings.ActrcFiles {
					components.Text(c, abbrevHome(file)).FontSize(t.Rem(11.0 / 13)).TextColor(t.TextMuted)
				}
			}
		}
	})
}
