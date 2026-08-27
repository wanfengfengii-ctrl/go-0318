package store

import "time"

// nowMs returns the current wall-clock instant in milliseconds. It is used only
// for bookkeeping timestamps; all domain logic uses an injected logical clock.
func nowMs() int64 { return time.Now().UnixMilli() }
