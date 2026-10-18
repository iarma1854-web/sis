package task

import (
	"errors"
	"testing"
)

func TestCreate_Success(t *testing.T) {
	svc := NewService(TextAnalyzer{})

	got, err := svc.Create("Hello Go world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("ID = %d, want 1", got.ID)
	}
	if got.State != Pending {
		t.Errorf("State = %s, want Pending", got.State)
	}
	if got.Result != nil {
		t.Errorf("Result = %+v, want nil before processing", got.Result)
	}
}

func TestCreate_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"whitespace only", " \t\n "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(TextAnalyzer{})
			_, err := svc.Create(tt.input)
			if !errors.Is(err, ErrEmptyInput) {
				t.Errorf("err = %v, want ErrEmptyInput", err)
			}
			if n := len(svc.List()); n != 0 {
				t.Errorf("invalid task was stored: %d tasks", n)
			}
		})
	}
}

func TestGet(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	created, _ := svc.Create("hello")

	t.Run("existing", func(t *testing.T) {
		got, err := svc.Get(created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != created.ID || got.Input != "hello" {
			t.Errorf("got %+v, want ID=%d Input=hello", got, created.ID)
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		_, err := svc.Get(999)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestProcess_Success(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	created, _ := svc.Create("Hello Go world")

	if err := svc.Process(created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.Get(created.ID)
	if got.State != Completed {
		t.Errorf("State = %s, want Completed", got.State)
	}
	if got.Result == nil {
		t.Fatal("Result is nil after processing")
	}
	want := Result{Words: 3, UniqueWords: 3, Runes: 14, Lines: 1}
	if *got.Result != want {
		t.Errorf("Result = %+v, want %+v", *got.Result, want)
	}
}

func TestTextAnalyzer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Result
	}{
		{"simple", "Hello Go world", Result{3, 3, 14, 1}},
		{"case insensitive unique", "Go go GO", Result{3, 1, 8, 1}},
		{"unicode", "привет мир", Result{2, 2, 10, 1}},
		{"multiline", "a\nb", Result{2, 2, 3, 2}},
		{"extra spaces", "  a   b  ", Result{2, 2, 9, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TextAnalyzer{}.Process(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestProcess_InvalidTransition(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	created, _ := svc.Create("hello")

	if err := svc.Process(created.ID); err != nil {
		t.Fatalf("first Process: %v", err)
	}
	err := svc.Process(created.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestProcess_NotFound(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	if err := svc.Process(42); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

type failingProcessor struct{}

func (failingProcessor) Process(string) (Result, error) {
	return Result{}, errors.New("boom")
}

func TestProcess_ProcessorFailure(t *testing.T) {
	svc := NewService(failingProcessor{})
	created, _ := svc.Create("hello")

	if err := svc.Process(created.ID); err == nil {
		t.Fatal("expected error, got nil")
	}
	got, _ := svc.Get(created.ID)
	if got.State != Failed {
		t.Errorf("State = %s, want Failed", got.State)
	}
	if got.Result != nil {
		t.Errorf("Result = %+v, want nil", got.Result)
	}
}

func TestMultipleIndependentTasks(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	a, _ := svc.Create("one two")
	b, _ := svc.Create("alpha beta gamma delta")
	c, _ := svc.Create("untouched")

	if a.ID == b.ID || b.ID == c.ID || a.ID == c.ID {
		t.Fatalf("IDs are not unique: %d %d %d", a.ID, b.ID, c.ID)
	}

	if err := svc.Process(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Process(b.ID); err != nil {
		t.Fatal(err)
	}

	ga, _ := svc.Get(a.ID)
	gb, _ := svc.Get(b.ID)
	gc, _ := svc.Get(c.ID)

	if ga.Result == nil || ga.Result.Words != 2 {
		t.Errorf("task a result = %+v, want 2 words", ga.Result)
	}
	if gb.Result == nil || gb.Result.Words != 4 {
		t.Errorf("task b result = %+v, want 4 words", gb.Result)
	}
	if gc.State != Pending || gc.Result != nil {
		t.Errorf("task c must be untouched, got state=%s result=%v", gc.State, gc.Result)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	svc := NewService(TextAnalyzer{})
	created, _ := svc.Create("hello world")
	_ = svc.Process(created.ID)

	got, _ := svc.Get(created.ID)
	got.State = Pending
	got.Result.Words = 100

	again, _ := svc.Get(created.ID)
	if again.State != Completed || again.Result.Words != 2 {
		t.Errorf("internal task was modified via returned copy: %+v", again)
	}
}
