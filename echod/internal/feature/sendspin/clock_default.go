//go:build !cube

package sendspin

import (
	ssync "github.com/Sendspin/sendspin-go/pkg/sync"
	"time"
)

func newSendspinClock() sessionClock { return ssync.NewClockSync() }
func micros() int64                  { return time.Now().UnixMicro() }
