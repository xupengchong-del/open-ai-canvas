package app

import (
	"testing"
	"time"
)

// TestPlaybackTranscodeLimit 覆盖环境变量解析：默认值、合法值、越界钳制、非法回退。
func TestPlaybackTranscodeLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"合法值", "3", 3},
		{"带空格", "  4 ", 4},
		{"零回退默认", "0", defaultPlaybackTranscodeLimit},
		{"负数回退默认", "-2", defaultPlaybackTranscodeLimit},
		{"非数字回退默认", "abc", defaultPlaybackTranscodeLimit},
		{"超大值钳制到上限", "999", maxPlaybackTranscodeLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(playbackTranscodeConcurrencyEnv, tc.raw)
			if got := playbackTranscodeLimit(); got != tc.want {
				t.Fatalf("playbackTranscodeLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestPlaybackTranscodeLimiterSingleton 验证闸口是单例，且容量受配置约束在合法范围。
// 单例保证所有派发路径（回填/上传）共用同一个闸口，才有全局并发上限的意义。
// 注意：Once 是包级单例，若其他测试已先初始化，容量以首次初始化为准，
// 因此这里只断言容量落在合法区间，不断言具体数值。
func TestPlaybackTranscodeLimiterSingleton(t *testing.T) {
	limiter := playbackTranscodeLimiter()
	if limiter == nil {
		t.Fatal("playbackTranscodeLimiter() 返回 nil")
	}
	if cap(limiter) <= 0 || cap(limiter) > maxPlaybackTranscodeLimit {
		t.Fatalf("闸口容量 = %d, 超出合法范围 [1, %d]", cap(limiter), maxPlaybackTranscodeLimit)
	}
	if again := playbackTranscodeLimiter(); again != limiter {
		t.Fatal("两次调用返回了不同闸口，非单例")
	}
}

// TestPlaybackTranscodeLimiterCapsConcurrency 验证真实闸口的并发上限语义：
// 占满所有槽位后，下一次获取必须阻塞，直到有槽位释放。
// 断电重启后正是这个机制阻止几十个 ffmpeg 同时启动打爆内存。
func TestPlaybackTranscodeLimiterCapsConcurrency(t *testing.T) {
	slots := playbackTranscodeLimiter()
	n := cap(slots)
	if n <= 0 {
		t.Fatalf("闸口容量非法: %d", n)
	}

	// 占满所有槽位（测试串行运行，进入时闸口应为空）。
	for i := 0; i < n; i++ {
		slots <- struct{}{}
	}
	// 确保测试结束释放全部槽位，避免影响同包其他测试。
	defer func() {
		for len(slots) > 0 {
			<-slots
		}
	}()

	acquired := make(chan struct{}, 1)
	go func() {
		slots <- struct{}{}
		acquired <- struct{}{}
	}()
	select {
	case <-acquired:
		t.Fatal("槽位已满时仍获取成功，并发闸口失效")
	case <-time.After(50 * time.Millisecond):
	}

	<-slots // 释放一个槽位
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("释放槽位后等待者未能获取")
	}
}
