package agi

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Answer answers the channel
func (a *AGI) Answer() error {
	return a.Command("ANSWER").Err()
}

// Status returns the channel status
func (a *AGI) Status() (State, error) {
	r, err := a.Command("CHANNEL STATUS").Res()
	if err != nil {
		return StateDown, err
	}
	state, err := strconv.Atoi(r)
	if err != nil {
		return StateDown, fmt.Errorf("failed to parse state %s: %w", r, err)
	}
	return State(state), nil
}

// Exec runs a dialplan application
func (a *AGI) Exec(cmd ...string) (string, error) {
	cmd = append([]string{"EXEC"}, cmd...)
	return a.Command(cmd...).Val()
}

// Get gets the value of the given channel variable
func (a *AGI) Get(key string) (string, error) {
	return a.Command("GET VARIABLE", key).Val()
}

// GetData plays a file and receives DTMF, returning the received digits
func (a *AGI) GetData(sound string, timeout time.Duration, maxDigits int) (digits string, err error) {
	if sound == "" {
		sound = "silence/1"
	}
	resp := a.CommandNoParse("GET DATA", sound, toMSec(timeout), strconv.Itoa(maxDigits))
	return resp.Res()
}

// Hangup terminates the call
func (a *AGI) Hangup() error {
	return a.Command("HANGUP").Err()
}

// RecordOptions describes the options available when recording
type RecordOptions struct {
	// Format is the format of the audio file to record; defaults to "wav".
	Format string

	// EscapeDigits is the set of digits on receipt of which will terminate the recording. Default is "#".  This may not be blank.
	EscapeDigits string

	// Timeout is the maximum time to allow for the recording.  Defaults to 5 minutes.
	Timeout time.Duration

	// Silence is the maximum amount of silence to allow before ending the recording.  The finest resolution is to the second.   0=disabled, which is the default.
	Silence time.Duration

	// Beep controls whether a beep is played before starting the recording.  Defaults to false.
	Beep bool

	// Offset is the number of samples in the recording to advance before storing to the file.  This is means of clipping the beginning of a recording.  Defaults to 0.
	Offset int
}

// Record records audio to a file
func (a *AGI) Record(name string, opts *RecordOptions) error {
	if opts == nil {
		opts = &RecordOptions{}
	}
	if opts.Format == "" {
		opts.Format = "wav"
	}
	if opts.EscapeDigits == "" {
		opts.EscapeDigits = "#"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	cmd := strings.Join([]string{
		"RECORD FILE ",
		name,
		opts.Format,
		opts.EscapeDigits,
		toMSec(opts.Timeout),
	}, " ")

	if opts.Offset > 0 {
		cmd += " " + strconv.Itoa(opts.Offset)
	}

	if opts.Beep {
		cmd += " BEEP"
	}

	if opts.Silence > 0 {
		cmd += " s=" + toSec(opts.Silence)
	}

	return a.Command(cmd).Err()
}

// SayAlpha plays a character string, annunciating each character.
func (a *AGI) SayAlpha(label string, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY ALPHA", label, escapeDigits).Val()
}

// SayDigits plays a digit string, annunciating each digit.
func (a *AGI) SayDigits(number string, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY DIGITS", number, escapeDigits).Val()
}

// SayDate plays a date
func (a *AGI) SayDate(when time.Time, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY DATE", toEpoch(when), escapeDigits).Val()
}

// SayDateTime plays a date using the given format.  See `voicemail.conf` for the format syntax; defaults to `ABdY 'digits/at' IMp`.
func (a *AGI) SayDateTime(when time.Time, escapeDigits string, format string) (digit string, err error) {
	// Extract the timezone from the time
	zone, _ := when.Zone()

	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}

	// Use the Asterisk default format if we are not given one
	if format == "" {
		format = "ABdY 'digits/at' IMp"
	}

	return a.Command("SAY DATETIME", toEpoch(when), escapeDigits, format, zone).Val()
}

// SayNumber plays the given number.
func (a *AGI) SayNumber(number string, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY NUMBER", number, escapeDigits).Val()
}

// SayPhonetic plays the given phrase phonetically
func (a *AGI) SayPhonetic(phrase string, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY PHOENTIC", phrase, escapeDigits).Val()
}

// SayTime plays the time part of the given timestamp
func (a *AGI) SayTime(when time.Time, escapeDigits string) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("SAY TIME", toEpoch(when), escapeDigits).Val()
}

// Set sets the given channel variable to
// the provided value.
func (a *AGI) Set(key, val string) error {
	return a.Command("SET VARIABLE", key, val).Err()
}

// SetRaw sets the given channel settings to
// the provided value.
func (a *AGI) SetRaw(key, val string) error {
	return a.Command("SET", key, val).Err()
}

// StreamFile plays the given file to the channel
func (a *AGI) StreamFile(name string, escapeDigits string, offset int) (digit string, err error) {
	// NOTE: AGI needs empty double quotes hold the place of the empty value in the line
	if escapeDigits == "" {
		escapeDigits = `""`
	}
	return a.Command("STREAM FILE", name, escapeDigits, strconv.Itoa(offset)).Val()
}

// Verbose logs the given message to the verbose message system
func (a *AGI) Verbose(msg string, level int) error {
	return a.Command("VERBOSE", strconv.Quote(msg), strconv.Itoa(level)).Err()
}

// Verbosef logs the formatted verbose output
func (a *AGI) Verbosef(format string, args ...interface{}) error {
	return a.Verbose(fmt.Sprintf(format, args...), 9)
}

// Log Sends an arbitrary text message to a selected log level
func (a *AGI) Log(logLevel, msg string) error {
	_, err := a.Exec("Log", strings.ToUpper(logLevel), msg)
	return err
}

func (a *AGI) LogError(msg string) error {
	return a.Log("ERROR", msg)
}

func (a *AGI) LogWarning(msg string) error {
	return a.Log("WARNING", msg)
}

func (a *AGI) LogNotice(msg string) error {
	return a.Log("NOTICE", msg)
}

func (a *AGI) LogDebug(msg string) error {
	return a.Log("DEBUG", msg)
}

func (a *AGI) LogVerbose(msg string) error {
	return a.Log("VERBOSE", msg)
}

func (a *AGI) LogDTMF(msg string) error {
	return a.Log("DTMF", msg)
}

// WaitForDigit waits for a DTMF digit and returns what is received
func (a *AGI) WaitForDigit(timeout time.Duration) (digit string, err error) {
	resp := a.Command("WAIT FOR DIGIT", toMSec(timeout))
	resp.ResultString = ""
	if resp.Error == nil && strconv.IsPrint(rune(resp.Result)) {
		resp.ResultString = strconv.Itoa(resp.Result)
	}
	return resp.Res()
}

// WaitForSilence waits for a specified amount of silence
func (a *AGI) WaitForSilence(silenceRequiredMsec int, iterations int, timeout time.Duration) (string, error) {
	execCmd := []string{
		"WaitForSilence",
		strconv.Itoa(silenceRequiredMsec),
		strconv.Itoa(iterations),
	}

	if timeout > 0 {
		execCmd = append(execCmd, toSec(timeout))
	}

	if _, err := a.Exec(execCmd...); err != nil {
		return "", err
	}

	return a.Get("WAITSTATUS")
}

// ExecPlayback plays back given filenames
func (a *AGI) ExecPlayback(filePath ...string) (string, error) {
	execCmd := []string{
		"Playback",
		strings.Join(filePath, "&"),
	}

	if _, err := a.Exec(execCmd...); err != nil {
		return "", err
	}

	return a.Get("PLAYBACKSTATUS")
}

// ExecBackground play a given audio filenames while waiting for digits of an extension to go to.
func (a *AGI) ExecBackground(filePath ...string) (string, error) {
	execCmd := []string{
		"BackGround",
		strings.Join(filePath, "&"),
	}

	if _, err := a.Exec(execCmd...); err != nil {
		return "", err
	}

	return a.Get("BACKGROUNDSTATUS")
}

// SetLogger setup external logger for low-level logging
func (a *AGI) SetLogger(l *zap.Logger) error {
	if l != nil && a.logger != nil {
		return errors.New("Logger already attached")
	}
	a.logger = l

	// Output variables
	if a.logger != nil {
		for k, v := range a.Variables {
			a.logger.Debug(fmt.Sprintf("$%s=%s\n", k, v))
		}
	}

	return nil
}

// ApplyLogger setup external logger for low-level logging
func (a *AGI) ApplyLogger(l *zap.Logger) {
	a.logger = l
}
