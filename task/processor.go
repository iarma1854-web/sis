package task

import (
	"strings"
	"unicode/utf8"
)

// Processor — интерфейс логики обработки. Сервис не зависит от конкретного анализатора.
type Processor interface {
	Process(input string) (Result, error)
}

// TextAnalyzer считает статистику текста.
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
