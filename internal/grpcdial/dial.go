// Package grpcdial opens only the explicitly configured pinned TLS target.
// It does not own sessions, user credentials, routing or owner operations.
package grpcdial

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
	"net/url"
	"time"
	"ucupa/shared/values"
)

type Config struct {
	NextHop          values.NextHop
	Expected         values.FactAuthorityRef
	Roots            *x509.CertPool
	SPKISHA256       [32]byte
	ConnectTimeout   time.Duration
	MaxMetadataBytes uint32
	Actor            values.FactAuthorityRef
	ActorCertificate *tls.Certificate
}

func Dial(c Config, interceptor grpc.UnaryClientInterceptor) (*grpc.ClientConn, error) {
	target, ok := c.NextHop.NetworkTarget()
	if !ok || !c.Expected.Valid() || c.Roots == nil || c.SPKISHA256 == ([32]byte{}) || c.ConnectTimeout <= 0 || c.MaxMetadataBytes < 256 || interceptor == nil {
		return nil, errors.New("invalid pinned target configuration")
	}
	if (c.Actor != "" || c.ActorCertificate != nil) && (!c.Actor.Valid() || c.ActorCertificate == nil || len(c.ActorCertificate.Certificate) == 0 || c.ActorCertificate.PrivateKey == nil) {
		return nil, errors.New("actor identity and client certificate required together")
	}
	u, _ := url.Parse(string(c.NextHop))
	config := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: c.Roots.Clone(), ServerName: u.Hostname(), NextProtos: []string{"h2"}}
	if c.ActorCertificate != nil {
		certificate := *c.ActorCertificate
		certificate.Certificate = make([][]byte, len(c.ActorCertificate.Certificate))
		for i, der := range c.ActorCertificate.Certificate {
			certificate.Certificate[i] = append([]byte(nil), der...)
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if state.NegotiatedProtocol != "h2" || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo) != c.SPKISHA256 {
			return errors.New("target authentication failed")
		}
		return nil
	}
	return grpc.NewClient("passthrough:///"+target, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithNoProxy(), grpc.WithDisableServiceConfig(), grpc.WithDisableRetry(), grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
		if address != target {
			return nil, errors.New("unexpected next hop")
		}
		ctx, cancel := context.WithTimeout(ctx, c.ConnectTimeout)
		defer cancel()
		return (&net.Dialer{}).DialContext(ctx, "tcp", target)
	}), grpc.WithMaxHeaderListSize(c.MaxMetadataBytes), grpc.WithUnaryInterceptor(interceptor))
}
