package gtkcord

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diamondburned/arikawa/state"
	"github.com/diamondburned/gtkcord3/log"
	"github.com/gotk3/gotk3/gtk"
	"github.com/pkg/errors"
)

type Login struct {
	*gtk.Box
	Token  *gtk.Entry
	Submit *gtk.Button
	Error  *gtk.Label

	// Button that opens discordlogin
	InfoButton *gtk.Button
}

func NewLogin() (*Login, error) {
	main, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	margin4(main, 15, 25, 25, 25)

	err, _ := gtk.LabelNew("")
	err.SetSingleLineMode(true)
	err.SetMarginBottom(10)

	token, _ := gtk.EntryNew()
	token.SetMarginBottom(15)
	token.SetInputPurpose(gtk.INPUT_PURPOSE_PASSWORD)
	token.SetPlaceholderText("Token")
	token.SetInvisibleChar('*')

	submit, _ := gtk.ButtonNewWithLabel("Login")
	info, _ := gtk.ButtonNewWithLabel("Use DiscordLogin")

	l := &Login{
		Token:      token,
		Submit:     submit,
		Error:      err,
		InfoButton: info,
	}

	submit.Connect("clicked", l.Login)

	main.Add(email)
	main.Add(password)
	main.Add(submit)

	return l, nil
}

func (l *Login) Login() {
	l.Box.SetSensitive(false)
	defer l.Box.SetSensitive(true)

	if err := l.login(); err != nil {
		l.Error.SetMarkup(fmt.Sprintf(
			`<span color="red">Error: %s</span>`,
			escape(strings.Title(err.Error())),
		))

		log.Errorln("Failed to login:", err)
		return
	}

	// TODO: init function
}

func (l *Login) login() error {
	token, err := l.Token.GetText()
	if err != nil {
		return errors.Wrap(err, "Failed to get text")
	}

	return l.tryToken(token)
}

func (l *Login) DiscordLogin() {
	l.Box.SetSensitive(false)
	defer l.Box.SetSensitive(true)
}

func (l *Login) discordLogin() error {
	path, err := LookPathExtras("discordlogin")
	if err != nil {
		return errors.Wrap(err, "DiscordLogin cannot be found")
	}

	cmd := &exec.Cmd{Path: path}
	cmd.Stderr = os.Stderr

	// UI will actually block during this time.

	b, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "DiscordLogin failed")
	}

	if len(b) == 0 {
		return errors.New("DiscordLogin returned nothing, check Console.")
	}

	return l.tryToken(string(b))
}

func (l *Login) tryToken(token string) error {
	s, err := state.New(token)
	if err != nil {
		return errors.Wrap(err, "Failed to create a new Discord session")
	}

	if err := s.Open(); err != nil {
		return errors.Wrap(err, "Failed to connect to Discord")
	}

	return nil
}

func LookPathExtras(file string) (string, error) {
	// Add extra PATHs, just in case:
	paths := filepath.SplitList(os.Getenv("PATH"))

	if gobin := os.Getenv("GOBIN"); gobin != "" {
		paths = append(paths, gobin)
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		paths = append(paths, gopath)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "go", "bin"))
	}

	const filename = "discordlogin"

	for _, dir := range paths {
		if dir == "" {
			dir = "."
		}

		path := filepath.Join(dir, filename)
		if err := findExecutable(path); err == nil {
			return path, nil
		}
	}

	return "", exec.ErrNotFound
}

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return os.ErrPermission
}
