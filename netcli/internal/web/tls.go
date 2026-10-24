package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/netcli/netcli/internal/config"
)

// EnsureCertificate returns a TLS certificate from etcDir, creating a
// self-signed one (valid 10 years) on first use. Replace web.crt/web.key
// with a real certificate if you have one.
func EnsureCertificate(etcDir, hostname string) (tls.Certificate, error) {
	certPath := filepath.Join(etcDir, "web.crt")
	keyPath := filepath.Join(etcDir, "web.key")
	if c, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if hostname == "" {
		hostname = "router"
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostname, Organization: []string{"netcli self-signed"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{hostname, "localhost"},
	}
	if ifaces, err := net.InterfaceAddrs(); err == nil {
		for _, a := range ifaces {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLinkLocalUnicast() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ipn.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
	if err := config.WriteFileAtomic(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := config.WriteFileAtomic(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// ListenAddr returns the listen address for the web configuration.
func ListenAddr(w config.WebConfig) string {
	port := w.Port
	if port == 0 {
		port = 80
		if w.TLS {
			port = 443
		}
	}
	return net.JoinHostPort(w.Bind, strconv.Itoa(port))
}
