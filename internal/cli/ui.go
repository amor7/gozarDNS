// Package cli is the `gozar` command: the setup wizard and the handful of
// commands an operator runs over ssh.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

var color = isTerminal(os.Stdout)

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
}

func paint(code, s string) string {
	if !color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string   { return paint("1", s) }
func dim(s string) string    { return paint("2", s) }
func accent(s string) string { return paint("38;5;141", s) }
func green(s string) string  { return paint("32", s) }
func yellow(s string) string { return paint("33", s) }
func red(s string) string    { return paint("31", s) }
func cyan(s string) string   { return paint("36", s) }

func banner() {
	fmt.Println()
	fmt.Println(accent("   ╭─╮ ╭─╮ ─╮ ╭─╮ ╭─╮"))
	fmt.Println(accent("   │ ┬ │ │ ╭╯ ├─┤ ├┬╯") + "   " + bold("Gozar"))
	fmt.Println(accent("   ╰─╯ ╰─╯ ╰─ ╵ ╵ ╵╰─") + "   " + dim("smart DNS gateway"))
	fmt.Println()
}

func step(n, total int, title string) {
	fmt.Printf("\n%s %s\n", accent(fmt.Sprintf("[%d/%d]", n, total)), bold(title))
}

func info(format string, a ...any) { fmt.Printf("   "+format+"\n", a...) }
func okf(format string, a ...any) {
	fmt.Printf("   %s "+format+"\n", append([]any{green("✓")}, a...)...)
}
func warnf(format string, a ...any) {
	fmt.Printf("   %s "+format+"\n", append([]any{yellow("!")}, a...)...)
}
func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{red("error:")}, a...)...)
}

// box prints lines inside a rounded frame.
func box(title string, lines []string) {
	w := utf8.RuneCountInString(title) + 4
	for _, l := range lines {
		w = max(w, visibleLen(l)+4)
	}
	fmt.Println()
	fmt.Println(accent("╭─ ") + bold(title) + " " + accent(strings.Repeat("─", w-visibleLen(title)-3)+"╮"))
	for _, l := range lines {
		fmt.Println(accent("│ ") + l + strings.Repeat(" ", w-visibleLen(l)-2) + accent("│"))
	}
	fmt.Println(accent("╰" + strings.Repeat("─", w) + "╯"))
}

// visibleLen ignores ANSI colour sequences.
func visibleLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- prompts

// The wizard is often run as `curl ... | sudo bash`, where stdin is the
// script itself. Questions are read from the terminal instead.
var in = openInput()

func openInput() *bufio.Reader {
	if !isTerminal(os.Stdin) {
		if tty, err := os.Open("/dev/tty"); err == nil {
			return bufio.NewReader(tty)
		}
	}
	return bufio.NewReader(os.Stdin)
}

var assumeYes = os.Getenv("GOZAR_YES") == "1"

func readLine() string {
	s, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return ""
	}
	return strings.TrimSpace(s)
}

func ask(q, def string) string {
	if assumeYes {
		return def
	}
	if def != "" {
		fmt.Printf("   %s %s %s ", cyan("?"), q, dim("["+def+"]"))
	} else {
		fmt.Printf("   %s %s ", cyan("?"), q)
	}
	if v := readLine(); v != "" {
		return v
	}
	return def
}

func confirm(q string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	if assumeYes {
		return def
	}
	fmt.Printf("   %s %s %s ", cyan("?"), q, dim("["+d+"]"))
	switch strings.ToLower(readLine()) {
	case "y", "yes", "بله", "آره":
		return true
	case "n", "no", "نه", "خیر":
		return false
	}
	return def
}

func choose(q string, options []string, def int) int {
	fmt.Printf("   %s %s\n", cyan("?"), q)
	for i, o := range options {
		mark := "  "
		if i == def {
			mark = accent("›")
		}
		fmt.Printf("     %s %d) %s\n", mark, i+1, o)
	}
	for {
		v := ask("choose", fmt.Sprint(def+1))
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil && n >= 1 && n <= len(options) {
			return n - 1
		}
	}
}
