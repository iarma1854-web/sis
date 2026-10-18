package main

import (
	"fmt"
	"log"

	"sis/task"
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
	fmt.Printf("created: id=%d state=%s\n", t.ID, t.State)

	if err := svc.Process(t.ID); err != nil {
		log.Fatal(err)
	}

	done, _ := svc.Get(t.ID)
	fmt.Printf("done: state=%s result=%+v\n", done.State, *done.Result)

	if err := svc.Process(t.ID); err != nil {
		fmt.Println("rejected:", err)
	}
}
