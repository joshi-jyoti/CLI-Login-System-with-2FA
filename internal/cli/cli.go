// Package cli implements the interactive command-line front end: an
// interactive prompt with history and tab-completion, before/after-login
// command sets, and user-friendly formatting of results and errors.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
	"golang.org/x/term"

	"cli-login-system/internal/service"
)

// session holds the small bit of client-side state the CLI keeps between
// commands: whether we're logged in, and if so, which session token to
// present to the service layer.
type session struct {
	loggedIn bool
	token    string
	username string
}

// App is the interactive CLI application.
type App struct {
	svc  *service.Service
	rl   *readline.Instance
	sess session
}

// preLoginCommands and postLoginCommands drive both help text and
// tab-completion.
var preLoginCommands = []string{"register", "login", "help", "exit"}
var postLoginCommands = []string{"whoami", "enable-2fa", "disable-2fa", "logout", "help", "exit"}

// New builds an App around the given service, wiring up readline with
// history and a completer that adapts to login state.
func New(svc *service.Service) (*App, error) {
	app := &App{svc: svc}

	completer := readline.NewPrefixCompleter()
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		HistoryFile:     historyFilePath(),
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return nil, fmt.Errorf("initializing readline: %w", err)
	}
	app.rl = rl
	app.refreshCompleter()
	return app, nil
}

// Close releases readline resources (and its history file handle).
func (a *App) Close() error {
	return a.rl.Close()
}

// Run starts the read-eval-print loop. It returns nil on normal exit
// (the "exit" command or Ctrl-D).
func (a *App) Run() error {
	fmt.Println("=== CLI Login System ===")
	fmt.Println(`Type "help" to see available commands.`)

	for {
		a.rl.SetPrompt(a.prompt())
		line, err := a.rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			// Ctrl-C: clear the current line and keep going.
			continue
		}
		if errors.Is(err, io.EOF) {
			fmt.Println("\nGoodbye!")
			return nil
		}
		if err != nil {
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if exit := a.dispatch(line); exit {
			fmt.Println("Goodbye!")
			return nil
		}
	}
}

func (a *App) prompt() string {
	if a.sess.loggedIn {
		return fmt.Sprintf("(%s)> ", a.sess.username)
	}
	return "> "
}

// dispatch runs a single command line and returns true if the app should
// exit.
func (a *App) dispatch(line string) (exit bool) {
	fields := strings.Fields(line)
	cmd := fields[0]

	switch cmd {
	case "exit", "quit":
		return true
	case "help":
		a.printHelp()
	case "register":
		if a.sess.loggedIn {
			fmt.Println("You are already logged in. Log out first to register a different account.")
			return false
		}
		a.cmdRegister()
	case "login":
		if a.sess.loggedIn {
			fmt.Println("You are already logged in.")
			return false
		}
		a.cmdLogin()
	case "whoami":
		a.requireLogin(a.cmdWhoAmI)
	case "enable-2fa":
		a.requireLogin(a.cmdEnable2FA)
	case "disable-2fa":
		a.requireLogin(a.cmdDisable2FA)
	case "logout":
		a.requireLogin(a.cmdLogout)
	default:
		fmt.Printf("Unknown command: %q. Type \"help\" for a list of commands.\n", cmd)
	}
	return false
}

// requireLogin runs fn only if a session is active; otherwise it prints a
// friendly error. This centralizes the auth-guard so individual command
// handlers can assume they're already authenticated.
func (a *App) requireLogin(fn func()) {
	if !a.sess.loggedIn {
		fmt.Println("You must be logged in to do that. Try \"login\" first.")
		return
	}
	fn()
}

func (a *App) printHelp() {
	fmt.Println()
	if a.sess.loggedIn {
		fmt.Println("Available commands:")
		fmt.Println("  whoami        Show your account details")
		fmt.Println("  enable-2fa    Enable TOTP-based two-factor authentication")
		fmt.Println("  disable-2fa   Disable two-factor authentication")
		fmt.Println("  logout        End your session")
		fmt.Println("  help          Show this message")
		fmt.Println("  exit          Quit the program")
	} else {
		fmt.Println("Available commands:")
		fmt.Println("  register      Create a new account")
		fmt.Println("  login         Log in to an existing account")
		fmt.Println("  help          Show this message")
		fmt.Println("  exit          Quit the program")
	}
	fmt.Println()
}

// refreshCompleter rebuilds tab-completion options to match the current
// login state, so a logged-out user doesn't get "whoami" suggested, etc.
func (a *App) refreshCompleter() {
	cmds := preLoginCommands
	if a.sess.loggedIn {
		cmds = postLoginCommands
	}
	items := make([]readline.PrefixCompleterInterface, 0, len(cmds))
	for _, c := range cmds {
		items = append(items, readline.PcItem(c))
	}
	a.rl.Config.AutoComplete = readline.NewPrefixCompleter(items...)
}

// readLine reads a single line of plain (echoed) input with the given
// prompt, e.g. for a username.
func (a *App) readLine(prompt string) (string, error) {
	a.rl.SetPrompt(prompt)
	defer a.rl.SetPrompt(a.prompt())
	line, err := a.rl.Readline()
	return strings.TrimSpace(line), err
}

// readSecret reads a line without echoing it to the terminal, for
// passwords. Falls back to visible input if the terminal doesn't support
// raw mode (e.g. when piping input in a test/non-interactive context).
func (a *App) readSecret(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		// Fall back to normal (visible) readline input.
		return a.readLine(prompt)
	}
	return strings.TrimSpace(string(b)), nil
}
