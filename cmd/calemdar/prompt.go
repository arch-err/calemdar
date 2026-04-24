package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// prompter is a small stdin-based Q&A helper. No external deps.
type prompter struct {
	r *bufio.Reader
	w io.Writer
}

func newPrompter() *prompter {
	return &prompter{r: bufio.NewReader(os.Stdin), w: os.Stdout}
}

// ask prints prompt and reads a line. Trims trailing newline and whitespace.
func (p *prompter) ask(prompt string) (string, error) {
	fmt.Fprint(p.w, prompt)
	line, err := p.r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// askDefault is like ask but returns def when input is empty.
func (p *prompter) askDefault(prompt, def string) (string, error) {
	s, err := p.ask(fmt.Sprintf("%s [%s]: ", prompt, def))
	if err != nil {
		return "", err
	}
	if s == "" {
		return def, nil
	}
	return s, nil
}

// askRequired loops until the user enters a non-empty string.
func (p *prompter) askRequired(prompt string) (string, error) {
	for {
		s, err := p.ask(prompt + ": ")
		if err != nil {
			return "", err
		}
		if s != "" {
			return s, nil
		}
		fmt.Fprintln(p.w, "  value required")
	}
}

// askChoice loops until the user enters one of options. Case-insensitive.
func (p *prompter) askChoice(prompt string, options []string) (string, error) {
	lower := make([]string, len(options))
	for i, o := range options {
		lower[i] = strings.ToLower(o)
	}
	for {
		s, err := p.ask(fmt.Sprintf("%s (%s): ", prompt, strings.Join(options, "/")))
		if err != nil {
			return "", err
		}
		s = strings.ToLower(s)
		for i, lo := range lower {
			if s == lo {
				return options[i], nil
			}
		}
		fmt.Fprintln(p.w, "  invalid choice")
	}
}

// askYN returns true for y/yes, false for n/no. def is the default on empty.
func (p *prompter) askYN(prompt string, def bool) (bool, error) {
	defStr := "y/N"
	if def {
		defStr = "Y/n"
	}
	for {
		s, err := p.ask(fmt.Sprintf("%s (%s): ", prompt, defStr))
		if err != nil {
			return false, err
		}
		s = strings.ToLower(s)
		if s == "" {
			return def, nil
		}
		switch s {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.w, "  y or n")
	}
}

// askInt loops until the user enters a valid integer (or empty → def).
func (p *prompter) askInt(prompt string, def int) (int, error) {
	for {
		s, err := p.ask(fmt.Sprintf("%s [%d]: ", prompt, def))
		if err != nil {
			return 0, err
		}
		if s == "" {
			return def, nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			fmt.Fprintln(p.w, "  not a number")
			continue
		}
		return n, nil
	}
}
