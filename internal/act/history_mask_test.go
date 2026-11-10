package act

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nektos/act/pkg/common"
	actrunner "github.com/nektos/act/pkg/runner"
	"github.com/sirupsen/logrus"
)

func TestHistoryMaskingMarker(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		r := &engine{onFlush: func() {}, level: logrus.InfoLevel, insecureSecrets: insecure}
		ctx := actrunner.WithJobLoggerFactory(context.Background(), r)
		masks := []string{"dynamic-secret"}
		//nolint:gosec // G101: these are synthetic secret values used to test masking.
		ctx = actrunner.WithJobLogger(ctx, "build", "ci/build", &actrunner.Config{InsecureSecrets: insecure, Secrets: map[string]string{"TOKEN": "configured-secret"}}, &masks, nil)
		common.Logger(ctx).Info("configured-secret dynamic-secret")
		// Direct logger messages do not pass through act's job masker.
		r.WithJobLogger().Info("configured-secret")
		lines, _, _ := r.Drain()
		for i, line := range lines {
			var event struct {
				Message string `json:"msg"`
				Masked  bool   `json:"msg_masked"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatal(err)
			}
			if event.Masked != (!insecure && i == 0) {
				t.Fatalf("incorrect masking guarantee: %s", line)
			}
			if event.Masked && strings.Contains(event.Message, "secret") {
				t.Fatalf("masked record contains secret: %s", line)
			}
		}
	}
}
