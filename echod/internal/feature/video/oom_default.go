//go:build !dot && !cube

package video

func oomAdjustment() string { return "echo 1000 > /proc/self/oom_score_adj && " }
