package task

import (
	"errors"
	"fmt"
	"strings"
)

// State описывает этап жизненного цикла задачи.
type State int

const (
	Pending State = iota
	Processing
	Completed
	Failed
)

func (s State) String() string {
	switch s {
	case Pending:
		return "Pending"
	case Processing:
		return "Processing"
	case Completed:
		return "Completed"
	case Failed:
		return "Failed"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

var (
	ErrEmptyInput        = errors.New("input is empty")
	ErrNotFound          = errors.New("task not found")
	ErrInvalidTransition = errors.New("invalid state transition")
)

// allowed хранит допустимые переходы между состояниями.
var allowed = map[State][]State{
	Pending:    {Processing},
	Processing: {Completed, Failed},
}

// Result — результат анализа текста.
type Result struct {
	Words       int
	UniqueWords int
	Runes       int
	Lines       int
}

// Task — единица работы: входной текст, состояние и результат.
type Task struct {
	ID     int
	Input  string
	State  State
	Result *Result // nil, пока задача не завершена
}

// transition меняет состояние, если переход разрешён.
func (t *Task) transition(to State) error {
	for _, s := range allowed[t.State] {
		if s == to {
			t.State = to
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.State, to)
}

// clone возвращает независимую копию задачи (включая Result).
func (t *Task) clone() Task {
	c := *t
	if c.Result != nil {
		r := *c.Result
		c.Result = &r
	}
	return c
}

// Validate проверяет входной текст.
func Validate(input string) error {
	if strings.TrimSpace(input) == "" {
		return ErrEmptyInput
	}
	return nil
}
