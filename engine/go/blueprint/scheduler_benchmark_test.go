package blueprint

import (
	"testing"
	"time"
)

func BenchmarkSharedTimerSchedulerScheduleCancel(b *testing.B) {
	scheduler := newSharedTimerScheduler()
	b.Cleanup(func() { _ = scheduler.Close() })
	callback := func() {}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		handle, err := scheduler.Schedule(time.Hour, callback)
		if err != nil {
			b.Fatal(err)
		}
		if !scheduler.Cancel(handle) {
			b.Fatal("scheduled task was not canceled")
		}
	}
}
