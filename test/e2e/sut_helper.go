package e2e

import (
	"bufio"
	"container/ring"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cometbft/cometbft/p2p"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WorkDir defines the default working directory for spawned processes.
var WorkDir = "."

// SystemUnderTest is used to manage processes and logs during test execution.
type SystemUnderTest struct {
	t *testing.T

	outBuff *ring.Ring
	errBuff *ring.Ring

	pidsLock sync.RWMutex
	pids     map[int]struct{}
}

// NewSystemUnderTest constructor
func NewSystemUnderTest(t *testing.T) *SystemUnderTest {
	r := &SystemUnderTest{
		t:       t,
		pids:    make(map[int]struct{}),
		outBuff: ring.New(100),
		errBuff: ring.New(100),
	}
	t.Cleanup(r.Shutdown)
	return r
}

// StartNode starts a process for the given command and manages it cleanup on test end.
func (s *SystemUnderTest) StartNode(cmd string, args ...string) {
	c := exec.Command( //nolint:gosec // used by tests only
		locateExecutable(cmd),
		args...,
	)
	c.Dir = WorkDir
	s.watchLogs(c)

	require.NoError(s.t, c.Start())

	// cleanup when stopped
	s.awaitProcessCleanup(c)
}

// AwaitNodeUp waits until a node is operational by validating it produces blocks.
func (s *SystemUnderTest) AwaitNodeUp(t *testing.T, rpcAddr string, timeout time.Duration) {
	t.Helper()
	t.Logf("Await node is up: %s", rpcAddr)
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()

	started := make(chan struct{}, 1)
	go func() { // query for a non empty block on status page
		t.Logf("Checking node status: %s\n", rpcAddr)
		for {
			con, err := rpchttp.New(rpcAddr, "/websocket")
			if err != nil || con.Start() != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			result, err := con.Status(ctx)
			if err != nil || result.SyncInfo.LatestBlockHeight < 1 {
				_ = con.Stop()
				time.Sleep(100 * time.Millisecond)
				continue
			}
			t.Logf("Node started. Current block: %d\n", result.SyncInfo.LatestBlockHeight)
			_ = con.Stop()
			started <- struct{}{}
			return
		}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		if !assert.NoError(t, ctx.Err()) {
			s.PrintBuffer()
			s.t.FailNow()
		}
	case <-time.NewTimer(timeout).C:
		s.PrintBuffer()
		t.Fatalf("timeout waiting for node start: %s", timeout)
	}
}

func (s *SystemUnderTest) awaitProcessCleanup(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	s.pidsLock.Lock()
	s.pids[pid] = struct{}{}
	s.pidsLock.Unlock()
	go func() {
		_ = cmd.Wait() // blocks until shutdown
		s.logf("Node stopped: %d\n", pid)
		s.pidsLock.Lock()
		delete(s.pids, pid)
		s.pidsLock.Unlock()
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
	s.outBuff.Do(func(v interface{}) {
		if v != nil {
			_, _ = fmt.Fprintf(out, "out> %s\n", v)
		}
	})
	_, _ = fmt.Fprint(out, "8< chain err -----------------------------------------\n")
	s.errBuff.Do(func(v interface{}) {
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

func (s *SystemUnderTest) logf(msg string, args ...interface{}) {
	s.log(fmt.Sprintf(msg, args...))
}

func (s *SystemUnderTest) hashPids() bool {
	s.pidsLock.RLock()
	defer s.pidsLock.RUnlock()
	return len(s.pids) != 0
}

func (s *SystemUnderTest) withEachPid(cb func(p *os.Process)) {
	s.pidsLock.RLock()
	pids := maps.Keys(s.pids)
	s.pidsLock.RUnlock()

	for pid := range pids {
		p, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		cb(p)
	}
}

// Shutdown stops all processes managed by the SystemUnderTest by sending SIGTERM and SIGKILL signals if necessary.
func (s *SystemUnderTest) Shutdown() {
	s.withEachPid(func(p *os.Process) {
		go func() {
			if err := p.Signal(syscall.SIGTERM); err != nil {
				s.logf("failed to stop node with pid %d: %s\n", p.Pid, err)
			}
		}()
	})
	for i := 0; i < 5; i++ {
		if !s.hashPids() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.withEachPid(func(p *os.Process) {
		s.logf("killing node %d\n", p.Pid)
		if err := p.Kill(); err != nil {
			s.logf("failed to kill node with pid %d: %s\n", p.Pid, err)
		}
	})
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
	node1Key, err := p2p.LoadOrGenNodeKey(filepath.Join(nodeDir, "config", "node_key.json"))
	require.NoError(t, err)
	p2pKey, err := crypto.UnmarshalEd25519PrivateKey(node1Key.PrivKey.Bytes())
	require.NoError(t, err)
	node1ID, err := peer.IDFromPrivateKey(p2pKey)
	require.NoError(t, err)
	return node1ID
}
