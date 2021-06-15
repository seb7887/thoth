package pool

import "github.com/segmentio/fasthash/fnv1a"

type WorkerPool struct {
	maxWorkers int
	taskQueue []chan func()
	stoppedChan chan struct{}
}

func New(maxWorkers int) *WorkerPool {
	// there must be at least one worker
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	// taskQueue is unbuffered since items are always removed immediately
	pool := &WorkerPool{
		taskQueue: make([]chan func(), maxWorkers),
		maxWorkers: maxWorkers,
		stoppedChan: make(chan struct{}),
	}

	// start the task dispatcher
	pool.dispatch()

	return pool
}

func (p *WorkerPool) Submit(uid string, task func()) {
	idx := fnv1a.HashString64(uid) % uint64(p.maxWorkers)
	if task != nil {
		p.taskQueue[idx] <-task
	}
}

func (p *WorkerPool) dispatch() {
	for i := 0; i < p.maxWorkers; i++ {
		p.taskQueue[i] = make(chan func(), 1024)
		go startWorker(p.taskQueue[i])
	}
}

func startWorker(taskChan chan func()) {
	go func() {
		var task func()
		var ok bool
		for {
			task, ok = <-taskChan
			if !ok {
				break
			}
			// execute the task
			task()
		}
	}()
}