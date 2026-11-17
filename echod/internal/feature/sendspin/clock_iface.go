package sendspin

import (
	ssync "github.com/Sendspin/sendspin-go/pkg/sync"
	"time"
)

type playbackClock interface {
	ServerToLocalTime(int64) time.Time
	ServerMicrosNow() int64
	CheckQuality() ssync.Quality
}

type sessionClock interface {
	playbackClock
	ProcessSyncResponse(int64, int64, int64, int64)
}
