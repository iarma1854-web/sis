package task

import (
	"strings"
	"unicode/utf8"
)

type Processor interface {
	Process(input string) (Result, error)
}

type TextAnalyzer struct{}

func (TextAnalyzer) Process(input string) (Result, error) {
	if err := Validate(input); err != nil {
		return Result{}, err
	}

	words := strings.Fields(input)
	unique := make(map[string]struct{}, len(words))
	for _, w := range words {
		unique[strings.ToLower(w)] = struct{}{}
	}

	return Result{
		Words:       len(words),
		UniqueWords: len(unique),
		Runes:       utf8.RuneCountInString(input),
		Lines:       strings.Count(strings.TrimSpace(input), "\n") + 1,
	}, nil
}
