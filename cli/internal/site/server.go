package site

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Build materializes the template, writes the bundle's content and runs
// astro build, which also builds the Pagefind search index. bun must have
// passed Preflight. out, when set, receives a copy of the built site.
func Build(ctx context.Context, w *Workdir, bun *Bun, out string, log io.Writer) (*Stats, error) {
	if _, err := w.Prepare(ctx, bun, log); err != nil {
		return nil, err
	}
	stats, err := Generate(w)
	if err != nil {
		return nil, fmt.Errorf("writing the site content: %w", err)
	}
	if err := w.run(ctx, bun, log, "--bun", "run", "build"); err != nil {
		return stats, fmt.Errorf("astro build failed: %w", err)
	}
	if _, err := os.Stat(filepath.Join(w.DistDir(), "pagefind")); err != nil {
		return stats, fmt.Errorf("the build produced no search index (dist/pagefind): %w", err)
	}
	if out != "" {
		if err := copyTree(w.DistDir(), out); err != nil {
			return stats, fmt.Errorf("copying the site to %s: %w", out, err)
		}
	}
	return stats, nil
}

// Server is a running preview server, as recorded by Astro's background
// lockfile (.astro/preview.json in the workdir).
type Server struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	URL       string `json:"url"`
	StartedAt string `json:"startedAt"`
}

// lockfile is where `astro preview --background` records its server.
func (w *Workdir) lockfile() string { return filepath.Join(w.Dir, ".astro", "preview.json") }

// Running returns the preview server recorded for this workdir, or nil when
// none is: no lockfile, an unreadable one, or a PID that is no longer this
// workdir's Astro server.
func (w *Workdir) Running() *Server {
	data, err := os.ReadFile(w.lockfile())
	if err != nil {
		return nil
	}
	var s Server
	if json.Unmarshal(data, &s) != nil || s.PID <= 0 {
		return nil
	}
	if !w.ownsProcess(s.PID) {
		return nil
	}
	return &s
}

// ownsProcess reports whether pid is alive and is the Astro server started in
// this workdir. On Linux the process's working directory must be the
// workdir, which is where Astro starts its background server; elsewhere its
// command line must run Astro's preview (from ps, or from CIM on Windows). A
// PID that cannot be attributed is never ours: PIDs are reused, and
// signalling a stranger is worse than leaving a server up.
func (w *Workdir) ownsProcess(pid int) bool {
	if !processAlive(pid) {
		return false
	}
	if runtime.GOOS == "linux" {
		cwd, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
		if err != nil {
			return false
		}
		want, _ := filepath.EvalSymlinks(w.Dir)
		if want == "" {
			want = w.Dir
		}
		got, _ := filepath.EvalSymlinks(cwd)
		if got == "" {
			got = cwd
		}
		if got != want {
			return false
		}
		cmdline, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
		return err == nil && strings.Contains(string(cmdline), "astro")
	}
	cmdline, err := processCommandLine(pid)
	return err == nil && strings.Contains(cmdline, "astro") && strings.Contains(cmdline, "preview")
}

// PortFree reports whether 127.0.0.1:port can be bound. Astro would otherwise
// move to another port without saying so, and `okfw site down` would then be
// asked to stop a server the user never meant to start.
func PortFree(port int) error {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("port %d is in use (%v); stop what holds it, or pass --port", port, err)
	}
	return l.Close()
}

// Up starts the preview server in the background on the built site. Pagefind
// search runs only on a built site — Starlight disables it under astro dev —
// which is why the site is served by astro preview, not the dev server.
func Up(ctx context.Context, w *Workdir, bun *Bun, port int, log io.Writer) (*Server, error) {
	if s := w.Running(); s != nil {
		return s, nil
	}
	if err := PortFree(port); err != nil {
		return nil, err
	}
	out, err := w.output(ctx, bun, "--bun", "run", "astro", "preview", "--background", "--port", strconv.Itoa(port))
	if err != nil {
		fmt.Fprint(log, out)
		return nil, fmt.Errorf("astro preview did not start: %w (log: %s)", err, filepath.Join(w.Dir, ".astro", "preview.log"))
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := w.Running(); s != nil {
			return s, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("astro preview reported success but recorded no server; see %s", filepath.Join(w.Dir, ".astro", "preview.log"))
}

// Down stops the preview server. It asks Astro first; when bun is gone (bun is
// nil) or Astro cannot stop it, it stops the process the lockfile names itself
// — but only once ownsProcess has attributed it to this workdir — so the port
// is freed whether or not bun still works. It reports the server it stopped,
// or nil when none was running.
func Down(ctx context.Context, w *Workdir, bun *Bun) (*Server, error) {
	s := w.Running()
	if s == nil {
		_ = os.Remove(w.lockfile()) // stale: its process is gone or not ours
		return nil, nil
	}
	if bun != nil {
		if _, err := w.output(ctx, bun, "--bun", "run", "astro", "preview", "stop"); err == nil && w.Running() == nil {
			return s, nil
		}
	}
	if err := stopProcess(s.PID); err != nil {
		return s, err
	}
	_ = os.Remove(w.lockfile())
	return s, nil
}
