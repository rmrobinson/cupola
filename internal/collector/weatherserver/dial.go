package weatherserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// dialCreds returns the gRPC dial option for the configured transport security.
// When useTLS is false, an insecure plaintext connection is used.
// When caCertPath is non-empty the PEM file is loaded and added as the sole
// trusted root, allowing connections to servers signed by a private CA.
func dialCreds(useTLS bool, caCertPath string) (grpc.DialOption, error) {
	if !useTLS {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}
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
	return grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)), nil
}
