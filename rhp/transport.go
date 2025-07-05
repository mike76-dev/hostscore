package rhp

import (
	"context"
	"net"

	"go.sia.tech/core/types"
	rhpv4 "go.sia.tech/coreutils/rhp/v4"
	"go.sia.tech/coreutils/rhp/v4/siamux"
)

// dial is a helper function, which connects to the specified address.
func dial(ctx context.Context, hostIP string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", hostIP)
	return conn, err
}

// WithTransportV4 creates a transport and calls an RHP4 RPC.
func WithTransportV4(ctx context.Context, addr string, hostKey types.PublicKey, fn func(rhpv4.TransportClient) error) (err error) {
	conn, err := dial(ctx, addr)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			conn.Close()
		}
	}()
	defer func() {
		close(done)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	t, err := siamux.Upgrade(ctx, conn, hostKey)
	if err != nil {
		return err
	}
	defer t.Close()
	return fn(t)
}
