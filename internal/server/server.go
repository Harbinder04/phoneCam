package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const maxFrameBytes = 4 << 20 // 4 MB

type Hub struct {
	token string
	mu    sync.RWMutex
	cond  *sync.Cond
	seq   int64
	frame []byte
	mime  string
}

type Server struct {
	http *http.Server
	hub  *Hub
}

func New(token string) (*Server, error) {
	if token == "" {
		return nil, errors.New("token cannot be empty")
	}

	hub := &Hub{token: token, mime: "image/jpeg"}
	hub.cond = sync.NewCond(&hub.mu)

	mux := http.NewServeMux()
	s := &Server{hub: hub}
	mux.HandleFunc("/", s.handlePhonePage)
	mux.HandleFunc("/api/frame", s.handleFrameUpload)
	mux.HandleFunc("/mjpeg", s.handleMJPEG)
	mux.HandleFunc("/health", s.handleHealth)

	s.http = &http.Server{
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

func (s *Server) ListenAndServe(addr string) error {
	s.http.Addr = addr
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handlePhonePage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token != s.hub.token {
		http.Error(w, "missing/invalid token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, phoneHTML(token))
}

func (s *Server) handleFrameUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("token") != s.hub.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxFrameBytes)
	defer body.Close()
	frame, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read frame: %v", err), http.StatusBadRequest)
		return
	}
	if len(frame) == 0 {
		http.Error(w, "empty frame", http.StatusBadRequest)
		return
	}

	mime := r.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}

	s.hub.mu.Lock()
	s.hub.frame = append(s.hub.frame[:0], frame...)
	s.hub.mime = mime
	s.hub.seq++
	s.hub.cond.Broadcast()
	s.hub.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMJPEG(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")

	last := int64(-1)
	for {
		frame, mime, seq, ok := s.hub.waitForFrame(r.Context(), last)
		if !ok {
			return
		}
		last = seq

		if _, err := fmt.Fprintf(w, "--frame\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n", mime, len(frame)); err != nil {
			return
		}
		if _, err := w.Write(frame); err != nil {
			return
		}
		if _, err := io.WriteString(w, "\r\n"); err != nil {
			return
		}
		flusher.Flush()
	}
}

func (h *Hub) waitForFrame(ctx context.Context, last int64) ([]byte, string, int64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for h.seq == last || len(h.frame) == 0 {
		if ctx.Err() != nil {
			return nil, "", 0, false
		}
		timer := time.AfterFunc(200*time.Millisecond, h.cond.Broadcast)
		h.cond.Wait()
		timer.Stop()
	}
	return bytes.Clone(h.frame), h.mime, h.seq, true
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func phoneHTML(token string) string {
	return `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>phonecam sender</title>
  <style>
    body { font-family: sans-serif; margin: 1rem; }
    video { width: 100%; border-radius: 12px; background: #111; }
    .row { display: flex; gap: .5rem; margin: .75rem 0; }
    button, input { padding: .6rem; font-size: 1rem; }
  </style>
</head>
<body>
  <h2>phonecam sender</h2>
  <p>Keep this page open to stream camera frames to your laptop.</p>
  <video id="v" autoplay playsinline muted></video>
  <div class="row">
    <label>FPS <input id="fps" type="number" min="1" max="30" value="12"></label>
    <button id="toggle">Start</button>
  </div>
  <small id="status">idle</small>

  <script>
    const token = ` + "`" + token + "`" + `;
    const v = document.getElementById('v');
    const fps = document.getElementById('fps');
    const status = document.getElementById('status');
    const canvas = document.createElement('canvas');
    let timer = null;

    async function initCam() {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: { ideal: 'environment' } },
        audio: false,
      });
      v.srcObject = stream;
      await v.play();
    }

    async function pushFrame() {
      if (!v.videoWidth || !v.videoHeight) return;
      canvas.width = v.videoWidth;
      canvas.height = v.videoHeight;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(v, 0, 0);
      const blob = await new Promise(res => canvas.toBlob(res, 'image/jpeg', 0.7));
      if (!blob) return;
      await fetch('/api/frame?token=' + encodeURIComponent(token), {
        method: 'POST',
        headers: { 'Content-Type': 'image/jpeg' },
        body: blob,
      });
      status.textContent = 'streaming ' + new Date().toLocaleTimeString();
    }

    document.getElementById('toggle').onclick = async (e) => {
      if (timer) {
        clearInterval(timer);
        timer = null;
        e.target.textContent = 'Start';
        status.textContent = 'stopped';
        return;
      }
      if (!v.srcObject) {
        try { await initCam(); } catch (err) {
          status.textContent = 'camera error: ' + err.message;
          return;
        }
      }
      const intervalMs = Math.max(33, 1000 / Math.max(1, Number(fps.value || 12)));
      timer = setInterval(() => pushFrame().catch(err => status.textContent = err.message), intervalMs);
      e.target.textContent = 'Stop';
      status.textContent = 'starting...';
    };
  </script>
</body>
</html>`
}
