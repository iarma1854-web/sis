package task

import (
	"fmt"
	"sort"
	"sync"
)

type Service struct {
	processor Processor

	mu      sync.RWMutex
	tasks   map[int]*Task
	nextID  int
	queue   chan int
	started bool
	stopped bool

	wg sync.WaitGroup
}

func NewService(p Processor) *Service {
	return &Service{
		processor: p,
		tasks:     make(map[int]*Task),
		nextID:    1,
	}
}

func (s *Service) Create(input string) (Task, error) {
	if err := Validate(input); err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	t := &Task{ID: s.nextID, Input: input, State: Pending}
	s.tasks[t.ID] = t
	s.nextID++
	return t.clone(), nil
}

func (s *Service) Get(id int) (Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return Task{}, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return t.clone(), nil
}

func (s *Service) List() []Task {
	s.mu.RLock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t.clone())
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) Process(id int) error {
	input, err := s.begin(id, Pending)
	if err != nil {
		return err
	}
	return s.finish(id, input)
}

func (s *Service) begin(id int, from State) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return "", fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	if t.State != from {
		return "", fmt.Errorf("task %d: %w: %s -> %s", id, ErrInvalidTransition, t.State, Processing)
	}
	if err := t.transition(Processing); err != nil {
		return "", fmt.Errorf("task %d: %w", id, err)
	}
	return t.Input, nil
}

func (s *Service) finish(id int, input string) error {
	res, perr := s.processor.Process(input)

	s.mu.Lock()
	defer s.mu.Unlock()

	t := s.tasks[id]
	if perr != nil {
		_ = t.transition(Failed)
		return fmt.Errorf("task %d: process: %w", id, perr)
	}
	t.Result = &res
	return t.transition(Completed)
}

func (s *Service) Start(workers, queueCap int) error {
	if workers < 1 || queueCap < 1 {
		return fmt.Errorf("%w: workers=%d queueCap=%d", ErrInvalidConfig, workers, queueCap)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return ErrAlreadyStarted
	}
	s.started = true
	s.queue = make(chan int, queueCap)

	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.worker(s.queue)
	}
	return nil
}

func (s *Service) worker(queue <-chan int) {
	defer s.wg.Done()
	for id := range queue {
		input, err := s.begin(id, Queued)
		if err != nil {
			continue
		}
		_ = s.finish(id, input)
	}
}

func (s *Service) Submit(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotRunning
	}
	if s.stopped {
		return ErrShutdown
	}
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	if t.State != Pending {
		return fmt.Errorf("task %d: %w: %s -> %s", id, ErrInvalidTransition, t.State, Queued)
	}

	select {
	case s.queue <- id:
		return t.transition(Queued)
	default:
		return fmt.Errorf("task %d: %w", id, ErrQueueFull)
	}
}

func (s *Service) Shutdown() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return ErrNotRunning
	}
	if !s.stopped {
		s.stopped = true
		close(s.queue)
	}
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}
