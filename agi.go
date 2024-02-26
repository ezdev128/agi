package agi

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// State describes the Asterisk channel state.  There are mapped
// directly to the Asterisk enumerations.
type State int

const (
	// StateDown indicates the channel is down and available
	StateDown State = iota

	// StateReserved indicates the channel is down but reserved
	StateReserved

	// StateOffhook indicates that the channel is offhook
	StateOffhook

	// StateDialing indicates that digits have been dialed
	StateDialing

	// StateRing indicates the channel is ringing
	StateRing

	// StateRinging indicates the channel's remote end is ringing (the channel is receiving ringback)
	StateRinging

	// StateUp indicates the channel is up
	StateUp

	// StateBusy indicates the line is busy
	StateBusy

	// StateDialingOffHook indicates digits have been dialed while offhook
	StateDialingOffHook

	// StatePreRing indicates the channel has detected an incoming call and is waiting for ring
	StatePreRing
)

// AGI represents an AGI session
type AGI struct {
	// Variables stored the initial variables
	// transmitted from Asterisk at the start
	// of the AGI session.
	Variables map[string]string

	r    io.Reader
	eagi io.Reader
	w    io.Writer

	conn net.Conn

	mu sync.Mutex

	// Logging ability
	logger *zap.Logger
}

// Response represents a response to an AGI
// request.
type Response struct {
	Error        error  // Error received, if any
	Status       int    // HTTP-style status code received
	Result       int    // Result is the numerical return (if parseable)
	ResultString string // Result value as a string
	Value        string // Value is the (optional) string value returned
}

// Res returns the ResultString of a Response, as well as any error encountered.  Depending on the command, this is sometimes more useful than Val()
func (r *Response) Res() (string, error) {
	return r.ResultString, r.Error
}

// Err returns the error value from the response
func (r *Response) Err() error {
	return r.Error
}

// Val returns the response value and error
func (r *Response) Val() (string, error) {
	return r.Value, r.Error
}

// Regex for AGI response result code and value
var responseRegex = regexp.MustCompile(`^(\d{3})\sresult=(-?[[:alnum:]]*)(\s.*)?$`)
var responseRegexNoParse = regexp.MustCompile(`^(\d{3})\sresult=(-?[[:alnum:]_*]*)(\s.*)?$`)
var responseRegexNoParseOtherResponse = regexp.MustCompile(`^(\d{3})\s([\s\w]+)$`)

const (
	// StatusOK indicates the AGI command was accepted.
	StatusOK = 200

	// StatusInvalid indicates Asterisk did not understand the command.
	StatusInvalid = 510

	// StatusDeadChannel indicates that the command cannot be performed on a dead (hangup) channel.
	StatusDeadChannel = 511

	// StatusEndUsage indicates...TODO
	StatusEndUsage = 520
)

// HandlerFunc is a function which accepts an AGI instance
type HandlerFunc func(*AGI)

// New creates an AGI session from the given reader and writer.
func New(r io.Reader, w io.Writer) *AGI {
	return NewWithEAGI(r, w, nil)
}

// NewWithEAGI returns a new AGI session to the given `os.Stdin` `io.Reader`,
// EAGI `io.Reader`, and `os.Stdout` `io.Writer`. The initial variables will
// be read in.
func NewWithEAGI(r io.Reader, w io.Writer, eagi io.Reader) *AGI {
	a := AGI{
		Variables: make(map[string]string),
		r:         r,
		w:         w,
		eagi:      eagi,
		logger:    zap.New(zapcore.NewNopCore()),
	}

	s := bufio.NewScanner(a.r)
	for s.Scan() {
		if s.Text() == "" {
			break
		}

		terms := strings.SplitN(s.Text(), ":", 2)
		if len(terms) == 2 {
			a.Variables[strings.TrimSpace(terms[0])] = strings.TrimSpace(terms[1])
		}
	}

	return &a
}

// NewConn returns a new AGI session bound to the given net.Conn interface
func NewConn(conn net.Conn) *AGI {
	a := New(conn, conn)
	a.conn = conn
	return a
}

// NewStdio returns a new AGI session to stdin and stdout.
func NewStdio() *AGI {
	return New(os.Stdin, os.Stdout)
}

// NewEAGI returns a new AGI session to stdin, the EAGI stream (FD=3), and stdout.
func NewEAGI() *AGI {
	return NewWithEAGI(os.Stdin, os.Stdout, os.NewFile(uintptr(3), "/dev/stdeagi"))
}

// Listen binds an AGI HandlerFunc to the given TCP `host:port` address, creating a FastAGI service.
func Listen(addr string, handler HandlerFunc) error {
	if addr == "" {
		addr = "localhost:4573"
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind server: %w", err)
	}
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)

	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept TCP connection: %w", err)
		}

		go handler(NewConn(conn))
	}
}

// Close closes any network connection associated with the AGI instance
func (a *AGI) Close() (err error) {
	if a.conn != nil {
		err = a.conn.Close()
		a.conn = nil
	}
	return
}

// EAGI enables access to the EAGI incoming stream (if available).
func (a *AGI) EAGI() io.Reader {
	return a.eagi
}

// Command sends the given command line to stdout
// and returns the response.
// TODO: this does not handle multi-line responses properly
func (a *AGI) Command(cmd ...string) (resp *Response) {
	resp = &Response{}
	cmdString := strings.Join(cmd, " ")
	var raw string

	a.mu.Lock()
	defer a.mu.Unlock()

	// Logging raw command and answer
	if a.logger != nil {
		defer func() {
			resString := ""
			if resp.Error == nil {
				resString += " Sta:" + strconv.Itoa(resp.Status)
				resString += " Res:" + strconv.Itoa(resp.Result)
				if resp.ResultString != "" {
					resString += " Str:" + resp.ResultString
				}
				if resp.Value != "" {
					resString += " Val:" + resp.Value
				}
			} else {
				resString += " Err:" + resp.Error.Error()
			}
			resString = "{" + strings.TrimSpace(resString) + "}"
			a.logger.Debug(fmt.Sprintf("#%s -> %s -> %s", cmdString, raw, resString))
		}()
	}

	_, err := a.w.Write([]byte(cmdString + "\n"))
	if err != nil {
		resp.Error = fmt.Errorf("failed to send command: %w", err)
		return resp
	}

	s := bufio.NewScanner(a.r)
	for s.Scan() {
		raw = s.Text()
		if raw == "" {
			break
		}

		if strings.HasPrefix(raw, "HANGUP") {
			resp.Error = ErrHangup
			return resp
		}

		// Parse and store the result code
		pieces := responseRegex.FindStringSubmatch(raw)
		if pieces == nil {
			resp.Error = fmt.Errorf("failed to parse result: %s", raw)
			return resp
		}

		// Status code is the first substring
		resp.Status, err = strconv.Atoi(pieces[1])
		if err != nil {
			resp.Error = fmt.Errorf("failed to get status code: %w", err)
			return resp
		}

		// Result code is the second substring
		resp.ResultString = pieces[2]
		resp.Result, err = strconv.Atoi(pieces[2])
		if err != nil {
			resp.Error = fmt.Errorf("failed to parse result-code as an integer: %w", err)
		}

		// Value is the third (and optional) substring
		wrappedVal := strings.TrimSpace(pieces[3])
		resp.Value = strings.TrimSuffix(strings.TrimPrefix(wrappedVal, "("), ")")

		// FIXME: handle multiple line return values
		break // nolint
	}

	// If the Status code is not 200, return an error
	if resp.Status != 200 {
		resp.Error = fmt.Errorf("non-200 status code")
	}

	return resp
}

// CommandNoParse sends the given command line to stdout
// and returns the response.
// TODO: this does not handle multi-line responses properly
func (a *AGI) CommandNoParse(cmd ...string) (resp *Response) {
	resp = &Response{}
	cmdString := strings.Join(cmd, " ")
	var raw string

	a.mu.Lock()
	defer a.mu.Unlock()

	// Logging raw command and answer
	if a.logger != nil {
		defer func() {
			resString := ""
			if resp.Error == nil {
				resString += " Sta:" + strconv.Itoa(resp.Status)
				resString += " Res:" + strconv.Itoa(resp.Result)
				if resp.ResultString != "" {
					resString += " Str:" + resp.ResultString
				}
				if resp.Value != "" {
					resString += " Val:" + resp.Value
				}
			} else {
				resString += " Err:" + resp.Error.Error()
			}
			resString = "{" + strings.TrimSpace(resString) + "}"
			a.logger.Debug(fmt.Sprintf("#%s -> %s -> %s", cmdString, raw, resString))
		}()
	}

	_, err := a.w.Write([]byte(cmdString + "\n"))
	if err != nil {
		resp.Error = fmt.Errorf("failed to send command: %w", err)
		return resp
	}

	s := bufio.NewScanner(a.r)
	for s.Scan() {
		raw = s.Text()
		if raw == "" {
			break
		}

		if strings.HasPrefix(raw, "HANGUP") {
			resp.Error = ErrHangup
			return resp
		}

		// Parse and store the result code
		pieces := responseRegexNoParse.FindStringSubmatch(raw)
		if pieces == nil {
			if responseRegexNoParseOtherResponse.MatchString(raw) {
				pieces = responseRegexNoParseOtherResponse.FindStringSubmatch(raw)
			} else {
				resp.Error = fmt.Errorf("failed to parse result: %s", raw)
				return resp
			}

		}

		// Status code is the first substring
		resp.Status, err = strconv.Atoi(pieces[1])
		if err != nil {
			resp.Error = fmt.Errorf("failed to get status code: %w", err)
			return resp
		}

		// Result code is the second substring
		resp.ResultString = pieces[2]

		if resp.Status == 511 {
			if strings.EqualFold(resp.ResultString, "Command Not Permitted on a dead channel or intercept routine") {
				resp.Error = Err511CommandNotPermitted
			} else {
				resp.Error = Err511GenericError
			}
			return resp
		}

		resp.Result, err = strconv.Atoi(pieces[2])
		if err != nil {
			resp.Result = 1
		}

		// Value is the third (and optional) substring
		wrappedVal := strings.TrimSpace(pieces[3])
		resp.Value = strings.TrimSuffix(strings.TrimPrefix(wrappedVal, "("), ")")

		if resp.Value == "timeout" {
			resp.Error = ErrTimeout
		}

		if resp.Status == 200 && resp.Value == "-1" {
			resp.Error = ErrHangup
			return resp
		}

		// FIXME: handle multiple line return values
		break // nolint
	}

	// If the Status code is not 200, return an error
	if resp.Status != 200 {
		resp.Error = fmt.Errorf("non-200 status code")
	}

	return resp
}
