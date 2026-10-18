package task

import (
	"fmt"
	"sort"
)

// Service хранит задачи в памяти и управляет их жизненным циклом.
type Service struct {
	processor Processor
	tasks     map[int]*Task
	nextID    int
}

func NewService(p Processor) *Service {
	return &Service{
		processor: p,
		tasks:     make(map[int]*Task),
		nextID:    1,
	}
}

// Create валидирует вход, создаёт задачу в состоянии Pending и сохраняет её.
func (s *Service) Create(input string) (Task, error) {
	if err := Validate(input); err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	t := &Task{ID: s.nextID, Input: input, State: Pending}
	s.tasks[t.ID] = t
	s.nextID++
	return t.clone(), nil
}

// Get возвращает копию задачи по ID.
func (s *Service) Get(id int) (Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return Task{}, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return t.clone(), nil
}

// Process выполняет задачу: Pending -> Processing -> Completed (или Failed).
func (s *Service) Process(id int) error {
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d: %w", id, ErrNotFound)
	}

	if err := t.transition(Processing); err != nil {
		return fmt.Errorf("task %d: %w", id, err)
	}

	res, err := s.processor.Process(t.Input)
	if err != nil {
		_ = t.transition(Failed) // из Processing переход всегда разрешён
		return fmt.Errorf("task %d: process: %w", id, err)
	}

	t.Result = &res
	return t.transition(Completed)
}

// List возвращает копии всех задач, отсортированные по ID.
func (s *Service) List() []Task {
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
