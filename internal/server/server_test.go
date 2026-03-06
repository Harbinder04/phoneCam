package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUnauthorizedWithoutToken(t *testing.T) {
	s, err := New("abc")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want %d got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUploadPublishesFrame(t *testing.T) {
	s, err := New("abc")
	if err != nil {
		t.Fatal(err)
	}

	frame := []byte("fakejpeg")
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/frame?token=abc", bytes.NewReader(frame))
	uploadReq.Header.Set("Content-Type", "image/jpeg")
	uploadW := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(uploadW, uploadReq)
	if uploadW.Code != http.StatusNoContent {
		t.Fatalf("upload status got %d", uploadW.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gotFrame, gotMime, _, ok := s.hub.waitForFrame(ctx, -1)
	if !ok {
		t.Fatal("waitForFrame returned not ok")
	}
	if string(gotFrame) != string(frame) {
		t.Fatalf("frame mismatch got %q want %q", string(gotFrame), string(frame))
	}
	if gotMime != "image/jpeg" {
		t.Fatalf("mime mismatch got %q", gotMime)
	}
}
