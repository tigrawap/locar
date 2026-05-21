package engine

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// runDeleteWorkers consumes per-directory batches and deletes their entries.
// Each worker takes one batch (the entries of a single directory) at a time and
// deletes them sequentially, so at most one delete is in flight per directory.
// Directories larger than the readdir batch size are split across batches and
// may briefly see one delete per in-flight batch. Worker count is --result-jobs.
//
// The workers depend only on deleteBatches being fed and eventually closed;
// they never wait on the traversal, so traversal backpressure on a full
// deleteBatches channel cannot deadlock. deleteBatches is closed by done()
// after doneDirectories, by which point every readdir goroutine has returned
// and no further send is possible.
func (e *Explorer) runDeleteWorkers(workers int) {
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range e.deleteBatches {
				for _, name := range batch {
					atomic.AddInt64(&e.activeDeletes, 1)
					var err error
					if e.deleteAll {
						err = os.RemoveAll(name)
					} else {
						err = os.Remove(name)
					}
					if err != nil {
						atomic.AddInt64(&e.totalDeleteFailed, 1)
						log.Printf("Delete failed: %s - Error: %v\n", name, err)
					} else {
						atomic.AddInt64(&e.totalDeleted, 1)
					}
					atomic.AddInt64(&e.activeDeletes, -1)
				}
			}
		}()
	}
	wg.Wait()
	e.printStats()
	close(e.deletesDone)
}

// deleteStatsLoop prints periodic progress until all deletes are done.
func (e *Explorer) deleteStatsLoop() {
	for {
		select {
		case <-e.deletesDone:
			return
		case <-time.After(1 * time.Second):
			e.printStats()
		}
	}
}

func (e *Explorer) printStats() {
	log.Printf("[stats] found: %d, active readdirs: %d, active deletes: %d, deleted: %d, failed: %d\n",
		atomic.LoadInt64(&e.found),
		atomic.LoadInt64(&e.debugInFlight),
		atomic.LoadInt64(&e.activeDeletes),
		atomic.LoadInt64(&e.totalDeleted),
		atomic.LoadInt64(&e.totalDeleteFailed))
}
