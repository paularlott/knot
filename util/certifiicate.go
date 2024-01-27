package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// GenerateCertificate generates a self-signed certificate and private key in PEM format.
func GenerateCertificate() (string, string, error) {
  priv, err := rsa.GenerateKey(rand.Reader, 2048)
  if err != nil {
    return "", "", err
  }

  // Set the certificate for 100 years.
  notBefore := time.Now().Add(-10 * time.Second)
  notAfter := notBefore.Add(3650 * 24 * time.Hour)

  template := x509.Certificate{
    SerialNumber: big.NewInt(1),
    Subject: pkix.Name{
      Organization: []string{"getknot.dev"},
			Country:      []string{"AU"},
    },
    NotBefore: notBefore,
    NotAfter:  notAfter,

    KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
    ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
    BasicConstraintsValid: true,
  }

  derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
  if err != nil {
    return "", "", err
  }

  certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
  keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

  return string(certPEM), string(keyPEM), nil
}