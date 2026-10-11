package livereload

import (
	"sort"
	"testing"
	"testing/synctest"
	"time"
)

func isReady(b *batcher) bool {
	select {
	case <-b.ready():
		return true
	default:
		return false
	}
}

func TestBatcher_FlushesAfterQuietPeriod(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBatcher(200*time.Millisecond, time.Second)
		defer b.stop()

		b.add("/a.md")
		time.Sleep(100 * time.Millisecond)
		b.add("/b.md") // restarts the quiet period
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		if isReady(b) {
			t.Fatal("batch flushed before the filesystem went quiet")
		}

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if !isReady(b) {
			t.Fatal("batch not flushed after the quiet period")
		}
		paths, reconcile := b.take()
		sort.Strings(paths)
		if len(paths) != 2 || paths[0] != "/a.md" || paths[1] != "/b.md" || reconcile {
			t.Errorf("take = %v, %v; want [/a.md /b.md], false", paths, reconcile)
		}
	})
}

func TestBatcher_MaxDelayCapsBusyWriters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBatcher(200*time.Millisecond, time.Second)
		defer b.stop()

		start := time.Now()
		for !isReady(b) {
			b.add("/busy.md")
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			if time.Since(start) > 2*time.Second {
				t.Fatal("busy writer starved the batch past the max delay")
			}
		}
		if elapsed := time.Since(start); elapsed < time.Second {
			t.Errorf("flushed after %v, before the 1s max delay", elapsed)
		}
	})
}

func TestBatcher_ReconcileRequestFlushes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBatcher(200*time.Millisecond, time.Second)
		defer b.stop()

		b.requestReconcile()
		b.add("/a.md")
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		if !isReady(b) {
			t.Fatal("reconcile request not flushed")
		}
		paths, reconcile := b.take()
		if !reconcile || len(paths) != 1 {
			t.Errorf("take = %v, %v; want [/a.md], true", paths, reconcile)
		}
	})
}

func TestBatcher_TakeDrainsAndStaysQuiet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBatcher(200*time.Millisecond, time.Second)
		defer b.stop()

		b.add("/a.md")
		b.add("/a.md")
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		if !isReady(b) {
			t.Fatal("not ready")
		}
		if paths, _ := b.take(); len(paths) != 1 {
			t.Errorf("duplicate adds not coalesced: %v", paths)
		}
		if paths, reconcile := b.take(); len(paths) != 0 || reconcile {
			t.Errorf("second take = %v, %v; want empty", paths, reconcile)
		}

		time.Sleep(2 * time.Second)
		synctest.Wait()
		if isReady(b) {
			t.Error("ready signalled with nothing pending")
		}
	})
}
