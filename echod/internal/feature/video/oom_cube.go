//go:build !dot && cube

package video

// The 2019 Lenovo kernel exposes oom_score_adj read-only to the unprivileged
// decoder. Keep the other resource limits even when this adjustment is absent.
func oomAdjustment() string {
	return "if [ -w /proc/self/oom_score_adj ]; then echo 1000 > /proc/self/oom_score_adj || exit 1; fi; "
}
