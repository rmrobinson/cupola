package weatherserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// dialOpts returns gRPC dial options for the configured transport security plus
// client-side keepalive. Keepalive detects silently-dead connections so that
// stream.Recv() returns an error and the caller can reconnect, rather than
// blocking forever after a server restart or network blip.
func dialOpts(useTLS bool, caCertPath string) ([]grpc.DialOption, error) {
	var credOpt grpc.DialOption
	if !useTLS {
		credOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		tlsCfg := &tls.Config{}
		if caCertPath != "" {
			pem, err := os.ReadFile(caCertPath)
			if err != nil {
				return nil, fmt.Errorf("read ca_cert %s: %w", caCertPath, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("ca_cert %s: no valid PEM certificates found", caCertPath)
			}
			tlsCfg.RootCAs = pool
		}
		credOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
	}
	kaOpt := grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	})
	return []grpc.DialOption{credOpt, kaOpt}, nil
}
