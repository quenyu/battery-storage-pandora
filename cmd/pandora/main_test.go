package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestShutdownFinishesActiveRequest(t *testing.T) {
	// Reserve a free address for this test, then let the server listen on it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Addr: address, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
			io.WriteString(w, "saved")
		case <-r.Context().Done():
		}
	})}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- runHTTPServer(ctx, server) }()

	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		deadline := time.Now().Add(2 * time.Second)
		for {
			response, err := client.Get("http://" + address)
			if err != nil {
				if time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				requestDone <- err
				return
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err == nil && (response.StatusCode != http.StatusOK || string(body) != "saved") {
				err = io.ErrUnexpectedEOF
			}
			requestDone <- err
			return
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()

	// Shutdown must close the listener while the active request is still running.
	deadline := time.Now().Add(2 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			break
		}
		connection.Close()
		if time.Now().After(deadline) {
			t.Fatal("server still accepts connections after cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-serverDone:
		t.Fatalf("server stopped before the active request completed: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("active request failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active request did not finish")
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish shutdown")
	}
}

func TestListenFailureReturnsWithoutWaitingForSignal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Addr: listener.Addr().String()}
	done := make(chan error, 1)
	go func() { done <- runHTTPServer(context.Background(), server) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "bind") {
			t.Fatalf("expected a bind error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup failure waited for a shutdown signal")
	}
}
