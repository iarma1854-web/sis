package task

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const timeout = 5 * time.Second

type gateProcessor struct {
	started chan struct{}
	release chan struct{}
	active  int32
	max     int32
}

func newGate() *gateProcessor {
	return &gateProcessor{
		started: make(chan struct{}, 100),
		release: make(chan struct{}),
	}
}

func (g *gateProcessor) Process(input string) (Result, error) {
	cur := atomic.AddInt32(&g.active, 1)
	for {
		m := atomic.LoadInt32(&g.max)
		if cur <= m || atomic.CompareAndSwapInt32(&g.max, m, cur) {
			break
		}
	}
	g.started <- struct{}{}
	<-g.release
	atomic.AddInt32(&g.active, -1)
	return TextAnalyzer{}.Process(input)
}

func (g *gateProcessor) waitStarted(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-g.started:
		case <-time.After(timeout):
			t.Fatalf("only %d/%d tasks started processing", i, n)
		}
	}
}

func mustCreate(t *testing.T, svc *Service, input string) Task {
	t.Helper()
	tk, err := svc.Create(input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return tk
}

func shutdown(t *testing.T, svc *Service) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- svc.Shutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(timeout):
		t.Fatal("Shutdown did not return: workers are stuck")
	}
}

func TestStart(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewService(TextAnalyzer{})
		if err := svc.Start(3, 5); err != nil {
			t.Fatalf("Start: %v", err)
		}
		shutdown(t, svc)
	})
	t.Run("twice", func(t *testing.T) {
		svc := NewService(TextAnalyzer{})
		if err := svc.Start(2, 2); err != nil {
			t.Fatal(err)
		}
		defer shutdown(t, svc)
		if err := svc.Start(2, 2); !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("err = %v, want ErrAlreadyStarted", err)
		}
	})
	t.Run("invalid config", func(t *testing.T) {
		svc := NewService(TextAnalyzer{})
		if err := svc.Start(0, 5); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("workers=0: err = %v, want ErrInvalidConfig", err)
		}
		if err := svc.Start(2, 0); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("queueCap=0: err = %v, want ErrInvalidConfig", err)
		}
	})
}

func TestSubmit_BeforeStart(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	tk := mustCreate(t, svc, "hello")
	if err := svc.Submit(tk.ID); !errors.Is(err, ErrNotRunning) {
		t.Errorf("err = %v, want ErrNotRunning", err)
	}
	if err := svc.Shutdown(); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Shutdown before Start: err = %v, want ErrNotRunning", err)
	}
}

func TestSubmit_ReturnsWithoutWaiting(t *testing.T) {
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(2, 2); err != nil {
		t.Fatal(err)
	}
	tk := mustCreate(t, svc, "hello world")

	if err := svc.Submit(tk.ID); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	g.waitStarted(t, 1)

	got, _ := svc.Get(tk.ID)
	if got.State != Processing {
		t.Errorf("State = %s, want Processing", got.State)
	}
	close(g.release)
	shutdown(t, svc)

	got, _ = svc.Get(tk.ID)
	if got.State != Completed {
		t.Errorf("State = %s, want Completed", got.State)
	}
}

func TestPool_ProcessesManyTasksWithCorrectResults(t *testing.T) {
	const n = 50
	svc := NewService(TextAnalyzer{})
	if err := svc.Start(4, n); err != nil {
		t.Fatal(err)
	}

	ids := make([]int, n)
	for i := 0; i < n; i++ {
		tk := mustCreate(t, svc, strings.TrimSpace(strings.Repeat("w ", i+1)))
		ids[i] = tk.ID
		if err := svc.Submit(tk.ID); err != nil {
			t.Fatalf("Submit %d: %v", tk.ID, err)
		}
	}
	shutdown(t, svc)

	for i, id := range ids {
		got, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != Completed {
			t.Errorf("task %d: State = %s, want Completed", id, got.State)
			continue
		}
		if got.Result == nil || got.Result.Words != i+1 {
			t.Errorf("task %d: Result = %+v, want %d words", id, got.Result, i+1)
		}
	}
}

func TestPool_TasksOverlapInTime(t *testing.T) {
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(2, 4); err != nil {
		t.Fatal(err)
	}
	a := mustCreate(t, svc, "a")
	b := mustCreate(t, svc, "b")
	_ = svc.Submit(a.ID)
	_ = svc.Submit(b.ID)

	g.waitStarted(t, 2)
	if got := atomic.LoadInt32(&g.active); got != 2 {
		t.Errorf("active = %d, want 2", got)
	}
	close(g.release)
	shutdown(t, svc)
}

func TestPool_ConcurrencyLimitedToWorkerCount(t *testing.T) {
	const workers = 3
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(workers, 20); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		tk := mustCreate(t, svc, fmt.Sprintf("task %d", i))
		if err := svc.Submit(tk.ID); err != nil {
			t.Fatal(err)
		}
	}
	g.waitStarted(t, workers)
	close(g.release)
	shutdown(t, svc)

	if m := atomic.LoadInt32(&g.max); m != workers {
		t.Errorf("max concurrent = %d, want exactly %d", m, workers)
	}
	for _, tk := range svc.List() {
		if tk.State != Completed {
			t.Errorf("task %d: State = %s, want Completed", tk.ID, tk.State)
		}
	}
}

func TestSubmit_QueueFull(t *testing.T) {
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(1, 1); err != nil {
		t.Fatal(err)
	}
	a := mustCreate(t, svc, "a")
	b := mustCreate(t, svc, "b")
	c := mustCreate(t, svc, "c")

	if err := svc.Submit(a.ID); err != nil {
		t.Fatal(err)
	}
	g.waitStarted(t, 1)
	if err := svc.Submit(b.ID); err != nil {
		t.Fatalf("Submit B: %v", err)
	}

	err := svc.Submit(c.ID)
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Submit C: err = %v, want ErrQueueFull", err)
	}
	gc, _ := svc.Get(c.ID)
	if gc.State != Pending || gc.Result != nil {
		t.Errorf("rejected task must stay Pending, got state=%s result=%v", gc.State, gc.Result)
	}
	gb, _ := svc.Get(b.ID)
	if gb.State != Queued {
		t.Errorf("B: State = %s, want Queued", gb.State)
	}

	close(g.release)

	deadline := time.After(timeout)
	for {
		err = svc.Submit(c.ID)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrQueueFull) {
			t.Fatalf("resubmit C: %v", err)
		}
		select {
		case <-deadline:
			t.Fatal("queue never freed up")
		default:
		}
	}
	shutdown(t, svc)

	for _, id := range []int{a.ID, b.ID, c.ID} {
		got, _ := svc.Get(id)
		if got.State != Completed {
			t.Errorf("task %d: State = %s, want Completed", id, got.State)
		}
	}
}

func TestSubmit_InvalidAndDuplicate(t *testing.T) {
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(1, 2); err != nil {
		t.Fatal(err)
	}

	if err := svc.Submit(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: err = %v, want ErrNotFound", err)
	}

	a := mustCreate(t, svc, "a")
	b := mustCreate(t, svc, "b")
	_ = svc.Submit(a.ID)
	g.waitStarted(t, 1)
	_ = svc.Submit(b.ID)

	if err := svc.Submit(a.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("submit Processing task: err = %v, want ErrInvalidTransition", err)
	}
	if err := svc.Submit(b.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("submit Queued task: err = %v, want ErrInvalidTransition", err)
	}
	if err := svc.Process(b.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Process Queued task: err = %v, want ErrInvalidTransition", err)
	}

	close(g.release)
	shutdown(t, svc)

	if err := svc.Process(a.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Process Completed task: err = %v, want ErrInvalidTransition", err)
	}
	ga, _ := svc.Get(a.ID)
	if ga.State != Completed || ga.Result == nil || ga.Result.Words != 1 {
		t.Errorf("A must be processed exactly once, got %+v", ga)
	}
}

func TestPool_ProcessorFailureMarksFailed(t *testing.T) {
	svc := NewService(failingProcessor{})
	if err := svc.Start(2, 2); err != nil {
		t.Fatal(err)
	}
	tk := mustCreate(t, svc, "hello")
	if err := svc.Submit(tk.ID); err != nil {
		t.Fatal(err)
	}
	shutdown(t, svc)

	got, _ := svc.Get(tk.ID)
	if got.State != Failed || got.Result != nil {
		t.Errorf("got state=%s result=%v, want Failed with nil result", got.State, got.Result)
	}
}

func TestConcurrentAccessToSharedData(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	if err := svc.Start(4, 100); err != nil {
		t.Fatal(err)
	}

	const producers, perProducer = 5, 10
	var created sync.Map
	var wg sync.WaitGroup

	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				tk, err := svc.Create(fmt.Sprintf("p%d item%d", p, i))
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				created.Store(tk.ID, true)
				if err := svc.Submit(tk.ID); err != nil {
					t.Errorf("Submit: %v", err)
				}
			}
		}(p)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 3; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, tk := range svc.List() {
					if g, err := svc.Get(tk.ID); err != nil || g.ID != tk.ID {
						t.Errorf("Get(%d) = %+v, %v", tk.ID, g, err)
						return
					}
				}
			}
		}()
	}

	wg.Wait()
	shutdown(t, svc)
	close(stop)
	readers.Wait()

	list := svc.List()
	if len(list) != producers*perProducer {
		t.Fatalf("len = %d, want %d", len(list), producers*perProducer)
	}
	for _, tk := range list {
		if tk.State != Completed || tk.Result == nil || tk.Result.Words != 2 {
			t.Errorf("task %d: %+v", tk.ID, tk)
		}
	}
}

func TestShutdown_RejectsNewSubmissionsAndIsIdempotent(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	if err := svc.Start(2, 2); err != nil {
		t.Fatal(err)
	}
	tk := mustCreate(t, svc, "late")

	shutdown(t, svc)
	if err := svc.Submit(tk.ID); !errors.Is(err, ErrShutdown) {
		t.Errorf("Submit after shutdown: err = %v, want ErrShutdown", err)
	}
	got, _ := svc.Get(tk.ID)
	if got.State != Pending {
		t.Errorf("State = %s, want Pending", got.State)
	}
	if err := svc.Shutdown(); err != nil {
		t.Errorf("second Shutdown: %v", err)
	}
	if err := svc.Start(2, 2); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("Start after shutdown: err = %v, want ErrAlreadyStarted", err)
	}
}

func TestShutdown_DrainsAcceptedTasks(t *testing.T) {
	g := newGate()
	svc := NewService(g)
	if err := svc.Start(1, 5); err != nil {
		t.Fatal(err)
	}
	var ids []int
	for i := 0; i < 6; i++ {
		tk := mustCreate(t, svc, fmt.Sprintf("job %d", i))
		ids = append(ids, tk.ID)
		if err := svc.Submit(tk.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			g.waitStarted(t, 1)
		}
	}

	done := make(chan error, 1)
	go func() { done <- svc.Shutdown() }()

	select {
	case <-done:
		t.Fatal("Shutdown returned while tasks were still unprocessed")
	case <-time.After(50 * time.Millisecond):
	}

	close(g.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(timeout):
		t.Fatal("Shutdown did not finish")
	}

	for _, id := range ids {
		got, _ := svc.Get(id)
		if got.State != Completed || got.Result == nil {
			t.Errorf("accepted task %d lost: %+v", id, got)
		}
	}
}

func TestShutdown_ConcurrentWithSubmit(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	if err := svc.Start(3, 10); err != nil {
		t.Fatal(err)
	}

	const n = 100
	ids := make([]int, n)
	for i := range ids {
		ids[i] = mustCreate(t, svc, "x y z").ID
	}

	accepted := make([]bool, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch err := svc.Submit(ids[i]); {
			case err == nil:
				accepted[i] = true
			case errors.Is(err, ErrQueueFull), errors.Is(err, ErrShutdown):
			default:
				t.Errorf("Submit: unexpected error %v", err)
			}
		}(i)
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.Shutdown(); err != nil {
				t.Errorf("Shutdown: %v", err)
			}
		}()
	}
	wg.Wait()

	for i, id := range ids {
		got, _ := svc.Get(id)
		if accepted[i] && got.State != Completed {
			t.Errorf("accepted task %d is %s, want Completed (lost task)", id, got.State)
		}
		if !accepted[i] && got.State != Pending {
			t.Errorf("rejected task %d is %s, want Pending", id, got.State)
		}
	}
}
