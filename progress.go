package uiprogress

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gosuri/uilive"
)

// Out is the default writer to render progress bars to
var Out = os.Stdout

// RefreshInterval in the default time duration to wait for refreshing the output
var RefreshInterval = time.Millisecond * 10

// defaultProgress is the default progress
var defaultProgress = New()

//split token to get size of terminal
var SizeToken = byte(' ')

var ErrExecFail = errors.New("errors: fail to get terminal width")

// Progress represents the container that renders progress bars
type Progress struct {
	// Out is the writer to render progress bars to
	Out io.Writer

	// Width is the width of the progress bars
	Width int

	// Bars is the collection of progress bars
	Bars []*Bar

	// RefreshInterval in the time duration to wait for refreshing the output
	RefreshInterval time.Duration

	//channel for sigwinch to change width
	sigChan chan os.Signal

	lw       *uilive.Writer
	stopChan chan struct{}
	mtx      *sync.RWMutex
}

// New returns a new progress bar with defaults
func New() *Progress {
	return &Progress{
		Width:           Width,
		Out:             Out,
		Bars:            make([]*Bar, 0),
		RefreshInterval: RefreshInterval,

		sigChan:  make(chan os.Signal, 1),
		lw:       uilive.New(),
		stopChan: make(chan struct{}),
		mtx:      &sync.RWMutex{},
	}
}

// AddBar creates a new progress bar and adds it to the default progress container
func AddBar(total int) *Bar {
	return defaultProgress.AddBar(total)
}

// Start starts the rendering the progress of progress bars using the DefaultProgress. It listens for updates using `bar.Set(n)` and new bars when added using `AddBar`
func Start() {
	defaultProgress.Start()
}

// Stop stops listening
func Stop() {
	defaultProgress.Stop()
}

// Listen listens for updates and renders the progress bars
func Listen() {
	defaultProgress.Listen()
}

// AddBar creates a new progress bar and adds to the container
func (p *Progress) AddBar(total int) *Bar {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	bar := NewBar(total)
	bar.Width = p.Width
	p.Bars = append(p.Bars, bar)
	return bar
}

func (p *Progress) ChangeWidth() {
	width, err := GetTerminalWidth()
	if err != nil {
		fmt.Println(err)
		return
	}

	p.mtx.RLock()
	for _, bar := range p.Bars {
		bar.SetWidth(width)
	}
	p.lw.Flush()
	p.mtx.RUnlock()
}

// Listen listens for updates and renders the progress bars
func (p *Progress) Listen() {
	p.lw.Out = p.Out
	for {
		select {
		case <-p.stopChan:
			return
		case <-p.sigChan:
			p.ChangeWidth()
		default:
			time.Sleep(p.RefreshInterval)
			p.mtx.RLock()
			for _, bar := range p.Bars {
				fmt.Fprintln(p.lw, bar.String())
			}
			p.lw.Flush()
			p.mtx.RUnlock()
		}
	}
}

// Start starts the rendering the progress of progress bars. It listens for updates using `bar.Set(n)` and new bars when added using `AddBar`
func (p *Progress) Start() {
	if p.stopChan == nil {
		p.stopChan = make(chan struct{})
	}

	if p.sigChan == nil {
		p.sigChan = make(chan os.Signal, 1)
	}
	p.SetNotify()
	go p.Listen()
}

// Stop stops listening
func (p *Progress) Stop() {
	close(p.stopChan)
	p.stopChan = nil
	close(p.sigChan)
	p.sigChan = nil
}

// Set Notify for syscall SIGWINCH to change width
func (p *Progress) SetNotify() {
	signal.Notify(p.sigChan, syscall.SIGWINCH)
}

func GetTerminalWidth() (int, error) {
	var option string
	switch runtime.GOOS {
	case "darwin":
		option = "-f"
	case "linux":
		option = "-F"
	default:
		option = "-f"
	}
	out, err := exec.Command(
		"stty",
		option,
		"/dev/tty",
		"size").Output()
	if err != nil {
		return 0, ErrExecFail
	}
	idx := bytes.IndexByte(out, SizeToken)
	width, _ := strconv.Atoi(string(out[idx+1 : len(out)-1]))
	return int(width) - 20, nil
}
