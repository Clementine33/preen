package coverhttp_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/coverhttp"
	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
)

func TestDownloadRejectsPrivateDNSConnections(t *testing.T) {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan struct{})
	go func() { defer close(done); answerPrivateDNS(server) }()
	previous := net.DefaultResolver
	//nolint:reassign // Route the production client through a controlled DNS server for this test.
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", server.LocalAddr().String())
	}}
	//nolint:reassign // Restore the process resolver before the test returns.
	defer func() { net.DefaultResolver = previous }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = coverhttp.New(nil).Download(ctx, "https://img.dlsite.jp/main.webp", coverdelivery.Validators{})
	if coverdelivery.Code(err) != "UNSUPPORTED_CDN" {
		t.Fatalf("private DNS address not blocked: %v", err)
	}
	if err = server.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
}

// The controlled resolver returns loopback answers for both A and AAAA queries.
func answerPrivateDNS(server net.PacketConn) {
	for {
		buf := make([]byte, 1024)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			return
		}
		raw := buf[:n]
		end := 12
		for end < len(raw) && raw[end] != 0 {
			end += int(raw[end]) + 1
		}
		end += 5
		if end > len(raw) || n < 12 {
			continue
		}
		response := append([]byte{raw[0], raw[1], 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0}, raw[12:end]...)
		answer := []byte{0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 1, 0, 4, 127, 0, 0, 1}
		if raw[end-3] == 28 {
			answer = []byte{0xc0, 0x0c, 0, 28, 0, 1, 0, 0, 0, 1, 0, 16}
			answer = append(answer, make([]byte, 16)...)
			answer[len(answer)-1] = 1
		}
		_, _ = server.WriteTo(append(response, answer...), peer)
	}
}
