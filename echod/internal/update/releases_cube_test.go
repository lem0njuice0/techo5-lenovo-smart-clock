//go:build cube && !dot && !spot && !panel

package update

import (
	"context"
	"strings"
	"testing"
)

func TestCubeRefusesOtherDeviceUpdates(t *testing.T) {
	if Published() || Stable.URL() != "" || Dev.URL() != "" {
		t.Fatal("cube must not advertise Echo releases")
	}
	if err := Install(context.Background(), Manifest{}, nil, nil); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsafe update not refused: %v", err)
	}
	if trial, reboot := Start(); trial || reboot {
		t.Fatalf("unexpected update startup %t %t", trial, reboot)
	}
}
