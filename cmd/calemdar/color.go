package main

import (
	"os"
)

// ANSI escape codes. Reset is 0; the rest are foreground colors or styles.
const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiGray    = "\x1b[90m" // bright black
	ansiCyan    = "\x1b[36m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
)

// colorOn is true iff stdout is a TTY and NO_COLOR is not set.
// Computed once at startup.
var colorOn = computeColorOn()

func computeColorOn() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// wrap applies codes around s, stripping when color is disabled.
func wrap(codes, s string) string {
	if !colorOn {
		return s
	}
	return codes + s + ansiReset
}

func gray(s string) string    { return wrap(ansiGray, s) }
func bold(s string) string    { return wrap(ansiBold, s) }
func cyan(s string) string    { return wrap(ansiCyan, s) }
func green(s string) string   { return wrap(ansiGreen, s) }
func yellow(s string) string  { return wrap(ansiYellow, s) }
func magenta(s string) string { return wrap(ansiMagenta, s) }
func dim(s string) string     { return wrap(ansiDim, s) }

// appName renders "calemdar" with the "md" dimmed to hint at the visual
// wordmark "cale**md**ar" without using asterisks.
func appName() string {
	return "cale" + gray("md") + "ar"
}
