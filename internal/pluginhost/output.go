//go:build unix

package pluginhost

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// maxHostOutputLineBytes caps one logged host output line. Longer lines are
// cut at the cap and the remainder is discarded up to the next newline, so a
// host cannot make the kernel buffer unbounded output.
const maxHostOutputLineBytes = 8 << 10

const hostOutputTruncatedMarker = " ...[truncated]"

// hostOutputPipes owns the stdout and stderr pipes of one host process. The
// kernel holds only the read ends after start; the readers finish when every
// process holding a write end (the host and its descendants) has exited.
type hostOutputPipes struct {
	stdoutRead, stdoutWrite *os.File
	stderrRead, stderrWrite *os.File
}

func newHostOutputPipes(command *exec.Cmd) (*hostOutputPipes, error) {
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		return nil, err
	}
	command.Stdout = stdoutWrite
	command.Stderr = stderrWrite
	return &hostOutputPipes{
		stdoutRead: stdoutRead, stdoutWrite: stdoutWrite,
		stderrRead: stderrRead, stderrWrite: stderrWrite,
	}, nil
}

// abort releases every descriptor when the host never started.
func (p *hostOutputPipes) abort() {
	p.closeWriteEnds()
	_ = p.stdoutRead.Close()
	_ = p.stderrRead.Close()
}

// forward closes the parent's write ends and logs each stream line by line
// until the child side closes. The readers are deliberately independent of
// exec.Cmd.Wait so a descendant holding a pipe open cannot delay exit
// detection.
func (p *hostOutputPipes) forward(prefix string, logf func(string, ...any)) {
	p.closeWriteEnds()
	go logHostOutput(p.stdoutRead, "["+prefix+" stdout]", logf)
	go logHostOutput(p.stderrRead, "["+prefix+" stderr]", logf)
}

func (p *hostOutputPipes) closeWriteEnds() {
	_ = p.stdoutWrite.Close()
	_ = p.stderrWrite.Close()
}

func hostOutputPrefix(ref ArtifactRef, generation uint64) string {
	return fmt.Sprintf("pkg:%s v:%s gen:%d", ref.PackageID, ref.Version, generation)
}

func logHostOutput(stream io.ReadCloser, prefix string, logf func(string, ...any)) {
	defer func() { _ = stream.Close() }()
	scanHostOutputLines(stream, func(line string) { logf("%s %s", prefix, line) })
}

// scanHostOutputLines calls emit for every line of stream. Memory is bounded
// by maxHostOutputLineBytes regardless of line length.
func scanHostOutputLines(stream io.Reader, emit func(string)) {
	reader := bufio.NewReaderSize(stream, maxHostOutputLineBytes)
	discarding := false
	for {
		chunk, err := reader.ReadSlice('\n')
		complete := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		switch {
		case discarding:
		case complete:
			emit(sanitizeHostOutputLine(chunk[:len(chunk)-1]))
		case errors.Is(err, bufio.ErrBufferFull):
			emit(sanitizeHostOutputLine(chunk) + hostOutputTruncatedMarker)
			discarding = true
		case len(chunk) > 0:
			// Final unterminated line before EOF or a read error.
			emit(sanitizeHostOutputLine(chunk))
		}
		if complete {
			discarding = false
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
}

// sanitizeHostOutputLine keeps one host line on one kernel log line: control
// characters other than tab are replaced so a host cannot forge log entries.
func sanitizeHostOutputLine(line []byte) string {
	text := strings.TrimSuffix(string(line), "\r")
	return strings.Map(func(character rune) rune {
		if character != '\t' && unicode.IsControl(character) {
			return unicode.ReplacementChar
		}
		return character
	}, text)
}
