package main

import (
	"errors"
	"fmt"
	"log"

	"sis/task"
)

const (
	workers  = 3
	queueCap = 5
)

func main() {
	svc := task.NewService(task.TextAnalyzer{})

	if _, err := svc.Create("   "); err != nil {
		fmt.Println("rejected:", err)
	}

	t, err := svc.Create("Hello Go world")
	if err != nil {
		log.Fatal(err)
	}
	if err := svc.Process(t.ID); err != nil {
		log.Fatal(err)
	}
	done, _ := svc.Get(t.ID)
	fmt.Printf("sync: id=%d state=%s result=%+v\n", done.ID, done.State, *done.Result)

	if err := svc.Start(workers, queueCap); err != nil {
		log.Fatal(err)
	}

	inputs := []string{"one two", "go go GO", "привет мир", "a\nb\nc", "the quick brown fox"}
	var ids []int
	for _, in := range inputs {
		tk, err := svc.Create(in)
		if err != nil {
			log.Fatal(err)
		}
		if err := svc.Submit(tk.ID); err != nil {
			if errors.Is(err, task.ErrQueueFull) {
				fmt.Printf("queue full, task %d stays %s\n", tk.ID, tk.State)
				continue
			}
			log.Fatal(err)
		}
		ids = append(ids, tk.ID)
	}

	if err := svc.Submit(ids[0]); err != nil {
		fmt.Println("rejected:", err)
	}

	if err := svc.Shutdown(); err != nil {
		log.Fatal(err)
	}

	for _, id := range ids {
		tk, _ := svc.Get(id)
		fmt.Printf("async: id=%d state=%s result=%+v\n", tk.ID, tk.State, *tk.Result)
	}

	late, _ := svc.Create("too late")
	if err := svc.Submit(late.ID); err != nil {
		fmt.Println("rejected:", err)
	}
}
