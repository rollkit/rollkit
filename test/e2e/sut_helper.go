package e2e

import (
	"bufio"
	"container/ring"
	"context"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/p2p/key"
	"github.com/evstack/ev-node/pkg/rpc/client"
	pb "github.com/evstack/ev-node/types/pb/evnode/v1"
)

// WorkDir defines the default working directory for spawned processes.
var WorkDir = "."

// SystemUnderTest is used to manage processes and logs during test execution.
type SystemUnderTest struct {
	t *testing.T

	outBuff *ring.Ring
	errBuff *ring.Ring

	pidsLock  sync.RWMutex
	pids      map[int]struct{}
	cmdToPids map[string][]int
	debug     bool
}

// NewSystemUnderTest constructor
func NewSystemUnderTest(t *testing.T) *SystemUnderTest {
	r := &SystemUnderTest{
		t:         t,
		pids:      make(map[int]struct{}),
		cmdToPids: make(map[string][]int),
		outBuff:   ring.New(100),
		errBuff:   ring.New(100),
	}
	t.Cleanup(r.ShutdownAll)
	return r
}

// RunCmd runs a command and returns the output
func (s *SystemUnderTest) RunCmd(cmd string, args ...string) (string, error) {
	c := exec.Command( //nolint:gosec // used by tests only
		locateExecutable(cmd),
		args...,
	)
	// Use CombinedOutput to capture both stdout and stderr
	combinedOutput, err := c.CombinedOutput()

	return string(combinedOutput), err
}

// ExecCmd starts a process for the given command and manages it cleanup on test end.
func (s *SystemUnderTest) ExecCmd(cmd string, args ...string) {
	executable := locateExecutable(cmd)
	c := exec.Command( //nolint:gosec // used by tests only
		executable,
		args...,
	)
	c.Dir = WorkDir
	s.watchLogs(c)

	err := c.Start()
	require.NoError(s.t, err)
	if s.debug {
		s.logf("Exec cmd (pid: %d): %s %s", c.Process.Pid, executable, strings.Join(c.Args, " "))
	}
	// cleanup when stopped
	s.awaitProcessCleanup(c)
}

// AwaitNodeUp waits until a node is operational by validating it produces blocks.
func (s *SystemUnderTest) AwaitNodeUp(t *testing.T, rpcAddr string, timeout time.Duration) {
	t.Helper()
	t.Logf("Await node is up: %s", rpcAddr)
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		c := client.NewClient(rpcAddr)
		require.NotNil(t, c)
		_, err := c.GetHealth(ctx)
		require.NoError(t, err)
	}, timeout, timeout/10, "node is not up")
}
func (s *SystemUnderTest) AwaitNBlocks(t *testing.T, n uint64, rpcAddr string, timeout time.Duration) {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()
	var c *client.Client
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		c = client.NewClient(rpcAddr)
		require.NotNil(t, c)
	}, timeout, 50*time.Millisecond, "client is not setup")
	var baseState *pb.State
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		s, err := c.GetState(ctx)
		require.NoError(t, err)
		baseState = s
	}, timeout, 50*time.Millisecond, "client is not setup")
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		s, err := c.GetState(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, s.LastBlockHeight, baseState.LastBlockHeight+n)
	}, timeout, 50*time.Millisecond, "client is not setup")
}

func (s *SystemUnderTest) awaitProcessCleanup(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	s.pidsLock.Lock()
	s.pids[pid] = struct{}{}
	cmdKey := filepath.Base(cmd.Path)
	s.cmdToPids[cmdKey] = append(s.cmdToPids[cmdKey], pid)
	s.pidsLock.Unlock()
	go func() {
		_ = cmd.Wait() // blocks until shutdown
		s.logf("Process stopped, pid: %d\n", pid)
		s.pidsLock.Lock()
		defer s.pidsLock.Unlock()
		delete(s.pids, pid)
		remainingPids := slices.DeleteFunc(s.cmdToPids[cmdKey], func(p int) bool { return p == pid })
		if len(remainingPids) == 0 {
			delete(s.cmdToPids, cmdKey)
		} else {
			s.cmdToPids[cmdKey] = remainingPids
		}
	}()
}

func (s *SystemUnderTest) watchLogs(cmd *exec.Cmd) {
	errReader, err := cmd.StderrPipe()
	if err != nil {
		panic(fmt.Sprintf("stderr reader error %#+v", err))
	}
	stopRingBuffer := make(chan struct{})
	go appendToBuf(errReader, s.errBuff, stopRingBuffer)

	outReader, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Sprintf("stdout reader error %#+v", err))
	}
	go appendToBuf(outReader, s.outBuff, stopRingBuffer)
	s.t.Cleanup(func() {
		close(stopRingBuffer)
	})
}

// PrintBuffer outputs the contents of outBuff and errBuff to stdout, prefixing each entry with "out>" or "err>", respectively.
func (s *SystemUnderTest) PrintBuffer() {
	out := os.Stdout
	s.outBuff.Do(func(v any) {
		if v != nil {
			_, _ = fmt.Fprintf(out, "out> %s\n", v)
		}
	})
	_, _ = fmt.Fprint(out, "8< chain err -----------------------------------------\n")
	s.errBuff.Do(func(v any) {
		if v != nil {
			_, _ = fmt.Fprintf(out, "err> %s\n", v)
		}
	})
}

func appendToBuf(r io.Reader, b *ring.Ring, stop <-chan struct{}) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-stop:
			return
		default:
		}
		text := scanner.Text()
		b.Value = text
		b = b.Next()
	}
}

func (s *SystemUnderTest) log(msg string) {
	s.t.Log(msg)
}

func (s *SystemUnderTest) logf(msg string, args ...any) {
	s.log(fmt.Sprintf(msg, args...))
}

func (s *SystemUnderTest) HasProcess(cmds ...string) bool {
	s.pidsLock.RLock()
	defer s.pidsLock.RUnlock()
	if len(cmds) == 0 {
		return len(s.pids) != 0
	}
	for _, cmd := range cmds {
		if len(s.cmdToPids[filepath.Base(cmd)]) != 0 {
			return true
		}
	}
	return false
}

// ShutdownAll stops all processes managed by the SystemUnderTest by sending SIGTERM and SIGKILL signals if necessary.
func (s *SystemUnderTest) ShutdownAll() {
	s.gracefulStopProcesses(s.iterAllProcesses)
}

// ShutdownByCmd stops all processes associated with the specified command by sending SIGTERM and SIGKILL if needed.
func (s *SystemUnderTest) ShutdownByCmd(cmd string) {
	s.gracefulStopProcesses(func() iter.Seq[*os.Process] { return s.iterProcessesByCmd(cmd) })
}

func (s *SystemUnderTest) gracefulStopProcesses(iterFn func() iter.Seq[*os.Process]) {
	for p := range iterFn() {
		go func(p *os.Process) {
			if err := p.Signal(syscall.SIGTERM); err != nil {
				s.logf("failed to stop node with pid %d: %s\n", p.Pid, err)
			}
		}(p)
	}

	// await graceful shutdown
	for range 5 {
		if !s.HasProcess() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// kill remaining processes if necessary
	for p := range iterFn() {
		s.logf("killing node %d\n", p.Pid)
		if err := p.Kill(); err != nil {
			s.logf("failed to kill node with pid %d: %s\n", p.Pid, err)
		}
	}
}

// iterAllProcesses returns an iterator over all processes currently managed by the SystemUnderTest instance.
func (s *SystemUnderTest) iterAllProcesses() iter.Seq[*os.Process] {
	return func(yield func(*os.Process) bool) {
		s.pidsLock.RLock()
		pids := maps.Keys(s.pids)
		s.pidsLock.RUnlock()

		for pid := range pids {
			p, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			if !yield(p) {
				break
			}
		}
	}
}

// iterProcessesByCmd returns an iterator over processes associated with the specified command.
func (s *SystemUnderTest) iterProcessesByCmd(cmd string) iter.Seq[*os.Process] {
	cmdKey := filepath.Base(cmd)
	return func(yield func(*os.Process) bool) {
		s.pidsLock.RLock()
		pids := slices.Clone(s.cmdToPids[cmdKey])
		s.pidsLock.RUnlock()

		for pid := range pids {
			p, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			if !yield(p) {
				break
			}
		}
	}
}

// locateExecutable looks up the binary on the OS path.
func locateExecutable(file string) string {
	if strings.TrimSpace(file) == "" {
		panic("executable binary name must not be empty")
	}
	path, err := exec.LookPath(file)
	if err != nil {
		panic(fmt.Sprintf("unexpected error with file %q: %s", file, err.Error()))
	}
	if path == "" {
		panic(fmt.Sprintf("%q not found", file))
	}
	return path
}

// MustCopyFile copies the file from the source path `src` to the destination path `dest` and returns an open file handle to `dest`.
func MustCopyFile(t *testing.T, src, dest string) *os.File {
	t.Helper()
	in, err := os.Open(src) // nolint: gosec // used by tests only
	require.NoError(t, err)
	defer in.Close() //nolint: errcheck // can be ignored

	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o750))

	out, err := os.Create(dest) // nolint: gosec // used by tests only
	require.NoError(t, err)
	defer out.Close() //nolint: errcheck // can be ignored

	_, err = io.Copy(out, in)
	require.NoError(t, err, "failed to copy from %q to %q: %v", src, dest, err)
	return out
}

// NodeID generates and returns the peer ID from the node's private key.
func NodeID(t *testing.T, nodeDir string) peer.ID {
	t.Helper()
	node1Key, err := key.LoadNodeKey(filepath.Join(nodeDir, "config"))
	require.NoError(t, err)
	node1ID, err := peer.IDFromPrivateKey(node1Key.PrivKey)
	require.NoError(t, err)
	return node1ID
}
