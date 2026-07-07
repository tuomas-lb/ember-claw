// Package mtls generates the CA and client certificates used to protect
// eclaw-managed web interfaces (dashboard, backlog, instance UIs) with nginx
// mutual-TLS client-certificate auth.
//
// The CA cert is what nginx verifies clients against (the `auth-tls-secret`);
// the client bundle (.p12) is imported into a browser / OS keychain. This keeps
// the whole "mTLS-protected interface" flow inside eclaw instead of ad-hoc
// openssl invocations.
package mtls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// CA holds a generated certificate authority.
type CA struct {
	Cert    *x509.Certificate
	CertPEM []byte
	KeyPEM  []byte
	key     *rsa.PrivateKey
}

// Client holds a generated client certificate signed by a CA.
type Client struct {
	CertPEM []byte
	KeyPEM  []byte
	// P12 is a PKCS#12 bundle (client cert + key + CA) for browser/OS import.
	P12 []byte
}

func serialNumber() (*big.Int, error) {
	// 128-bit random serial.
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

// NewCA creates a self-signed CA valid for `days` days. commonName/org label it.
func NewCA(commonName, org string, days int) (*CA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{org}},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().AddDate(0, 0, days),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{
		Cert:    cert,
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		key:     key,
	}, nil
}

// NewClient issues a client certificate (CN=commonName) signed by the CA and
// packages it as a PKCS#12 bundle protected by p12Password (may be empty).
func (ca *CA) NewClient(commonName, org string, days int, p12Password string) (*Client, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate client key: %w", err)
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{org}},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().AddDate(0, 0, days),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign client cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	// Legacy encoding for broad browser/OS keychain compatibility.
	p12, err := pkcs12.Legacy.Encode(key, cert, []*x509.Certificate{ca.Cert}, p12Password)
	if err != nil {
		return nil, fmt.Errorf("encode pkcs12: %w", err)
	}
	return &Client{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		P12:     p12,
	}, nil
}
