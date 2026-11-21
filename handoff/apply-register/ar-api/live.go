package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// sse streams events until the client goes; `every` produces the next event, nil to send nothing this tick.
func sse(w http.ResponseWriter, r *http.Request, period time.Duration, every func(n int) any) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "no_stream", "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	tick := time.NewTicker(period)
	defer tick.Stop()
	for n := 0; ; n++ {
		if ev := every(n); ev != nil {
			body, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: message\ndata: %s\n\n", n, body)
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

func (a *api) inbox(w http.ResponseWriter, r *http.Request) {
	u := a.user(w, r)
	if u == nil {
		return
	}
	titles := []string{"Seats are going fast", "A new organisation near you", "Your registration moved forward", "Reminder: event tomorrow"}
	sse(w, r, 20*time.Second, func(n int) any {
		if n == 0 {
			return nil
		}
		a.db.mu.Lock()
		defer a.db.mu.Unlock()
		nt := Notice{ID: a.db.next("ntc"), Title: titles[n%len(titles)], Body: "Tap to see what changed.", Route: "ex-org", Params: map[string]string{"slug": []string{"cityrun", "devconf", "arena", "campus"}[n%4]}, At: iso(time.Now().UTC())}
		a.db.notices = append(a.db.notices, nt)
		return nt
	})
}

// queue: the clinic's queue for a ticket, moving one place every 5 s until it is your turn.
func (a *api) queue(w http.ResponseWriter, r *http.Request) {
	start := 6
	if t := r.URL.Query().Get("ticket"); t != "" {
		start = 4 + len(t)%5
	}
	sse(w, r, 5*time.Second, func(n int) any {
		pos := max(start-n, 0)
		status := "waiting"
		switch {
		case pos == 0:
			status = "called"
		case pos <= 2:
			status = "soon"
		}
		return map[string]any{"position": pos, "waitMinutes": pos * 4, "status": status}
	})
}

// registrationLive: submitted → reviewing → approved over a minute.
func (a *api) registrationLive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	steps := []struct{ status, note string }{{"submitted", "We received it."}, {"reviewing", "Someone is looking at it now."}, {"approved", "You are in. Your pass is ready."}}
	sse(w, r, 20*time.Second, func(n int) any {
		s := steps[min(n, len(steps)-1)]
		if n >= len(steps) {
			return nil
		}
		return map[string]any{"id": id, "status": s.status, "note": s.note}
	})
}

// runner: a WebSocket that sends a simulated runner on the river route every 3 s (RFC 6455, text frames only).
func (a *api) runner(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "websocket" {
		fail(w, http.StatusUpgradeRequired, "websocket", "open this as a WebSocket")
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	hj, ok := w.(http.Hijacker)
	if !ok {
		fail(w, http.StatusInternalServerError, "no_hijack", "websocket unsupported")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
	_ = rw.Flush()
	done := make(chan struct{})
	go func() { // read until the client closes; its frames are not needed
		defer close(done)
		br := bufio.NewReader(conn)
		for {
			head := make([]byte, 2)
			if _, err := br.Read(head); err != nil || head[0]&0x0f == 0x8 {
				return
			}
			n := int(head[1] & 0x7f)
			switch n {
			case 126:
				ext := make([]byte, 2)
				_, _ = br.Read(ext)
				n = int(binary.BigEndian.Uint16(ext))
			case 127:
				ext := make([]byte, 8)
				_, _ = br.Read(ext)
				n = int(binary.BigEndian.Uint64(ext))
			}
			if head[1]&0x80 != 0 {
				n += 4
			}
			_, _ = br.Discard(n)
		}
	}()
	bib, _ := strconv.Atoi(r.PathValue("bib"))
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for n := 0; ; n++ {
		km := math.Min(float64(n)*0.18+float64(bib%5), 42.2)
		lat := 52.5200 + 0.02*math.Sin(km/6)
		lng := 13.4050 + km*0.004
		status := "running"
		if km >= 42.2 {
			status = "finished"
		}
		msg, _ := json.Marshal(map[string]any{"lat": lat, "lng": lng, "km": math.Round(km*10) / 10, "etaMinutes": int(math.Max(0, (42.2-km)*5.2)), "status": status})
		if writeText(conn, msg) != nil {
			return
		}
		select {
		case <-done:
			return
		case <-tick.C:
		}
	}
}

func writeText(conn interface{ Write([]byte) (int, error) }, msg []byte) error {
	frame := []byte{0x81}
	switch n := len(msg); {
	case n < 126:
		frame = append(frame, byte(n))
	case n < 65536:
		frame = append(frame, 126, byte(n>>8), byte(n))
	default:
		frame = append(frame, 127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(n))
	}
	_, err := conn.Write(append(frame, msg...))
	return err
}
