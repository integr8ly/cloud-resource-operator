package resources

import (
	"context"
	"net/http"
	"testing"
)

func TestConnectionTestManager_TCPConnection(t *testing.T) {
	type args struct {
		host string
		port int
	}
	tests := []struct {
		name     string
		args     args
		expected bool
	}{
		{
			name: "successfully established connection",
			args: args{
				host: "localhost",
				port: 8008,
			},
			expected: true,
		},
		{
			name: "failed to establish connection",
			args: args{
				host: "localhost",
				port: 6666,
			},
			expected: false,
		},
	}
	// start a new server
	server := &http.Server{Addr: ":8008"}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			t.Errorf("error starting test server: %v", err)
			return
		}
	}()
	defer func() {
		if err := server.Shutdown(context.TODO()); err != nil {
			t.Fatalf("error shutting down test server: %v", err)
		}
	}()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctm := NewConnectionTestManager()
			if got := ctm.TCPConnection(tt.args.host, tt.args.port); got != tt.expected {
				t.Errorf("TCPConnection() = %v, want %v", got, tt.expected)
			}
		})
	}
}
