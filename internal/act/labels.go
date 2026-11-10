package act

import (
	"slices"
	"strings"

	"github.com/nektos/act/pkg/model"
)

// UnmappedLabels returns, in plan order, the runner label act would fail to
// resolve for each job that is about to run: the first label of every job
// where none of its runs-on labels has an image in platforms ("runner=image"
// values) or act's built-in fallbacks. Labels are lowercased as act does and
// expression labels (${{ }}) are skipped, since they cannot be resolved
// statically. Workflows that cannot be planned are left for act to report.
func UnmappedLabels(dir string, workflows []string, event, job string, platforms []string) []string {
	known := builtinPlatforms()
	for _, p := range platforms {
		if name, image, ok := strings.Cut(p, "="); ok && image != "" {
			known[strings.ToLower(strings.TrimSpace(name))] = image
		}
	}
	if len(workflows) == 0 {
		workflows = []string{".github/workflows"}
	}
	if event == "" {
		event = "push"
	}
	var missing []string
	for _, w := range workflows {
		planner, err := model.NewWorkflowPlanner(resolvePath(dir, w), true, false)
		if err != nil {
			continue
		}
		var plan *model.Plan
		if job != "" {
			plan, err = planner.PlanJob(job)
		} else {
			plan, err = planner.PlanEvent(event)
		}
		if err != nil || plan == nil {
			continue
		}
		for _, stage := range plan.Stages {
			for _, run := range stage.Runs {
				if label := unmappedLabel(run.Job().RunsOn(), known); label != "" && !slices.Contains(missing, label) {
					missing = append(missing, label)
				}
			}
		}
	}
	return missing
}

// unmappedLabel returns the first label of a job when none of its labels is
// mapped; act runs the job on the first mapped label and skips it otherwise.
func unmappedLabel(labels []string, known map[string]string) string {
	first := ""
	for _, label := range labels {
		label = strings.ToLower(label)
		if strings.Contains(label, "${{") {
			return ""
		}
		if known[label] != "" {
			return ""
		}
		if first == "" {
			first = label
		}
	}
	return first
}
