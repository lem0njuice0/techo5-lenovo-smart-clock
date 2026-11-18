//go:build !dot && cube

package video

import (
	"strings"
	"testing"
)

func TestCubeDecoderKeepsLimitsWhenOOMAdjustmentUnavailable(t *testing.T) {
	_, args := wrap("/decoder", nil)
	script := args[1]
	if !strings.Contains(script, "if [ -w /proc/self/oom_score_adj ]") ||
		!strings.Contains(script, "ulimit -v 655360 &&") ||
		!strings.Contains(script, "ulimit -n 64 &&") ||
		!strings.Contains(script, `exec nice -n 5 "$0" "$@"`) {
		t.Fatalf("unexpected decoder guard: %s", script)
	}
}
