package evolution

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
)

// signPackage signs the package payload with the given private key and
// returns the fully populated package. The signature covers the canonical
// payload (metadata + proposals + public key, excluding sha256/signature).
func signPackage(p ExperiencePackage, privateKey []byte) (ExperiencePackage, error) {
	priv, err := parsePrivateKey(privateKey)
	if err != nil {
		return ExperiencePackage{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	p.SignatureAlgorithm = SignatureAlgorithmEd25519
	p.PublicKey = hex.EncodeToString(pub)
	p.SHA256 = packageHash(p)
	sig := ed25519.Sign(priv, p.signablePayload())
	p.Signature = hex.EncodeToString(sig)
	return p, nil
}

// VerifySignature checks the embedded signature against the embedded public
// key. Returns false for unsigned packages.
func (p ExperiencePackage) VerifySignature() bool {
	if !p.IsSigned() || p.SignatureAlgorithm != SignatureAlgorithmEd25519 {
		return false
	}
	pub, err := hex.DecodeString(p.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(p.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), p.signablePayload(), sig)
}

// verifyAgainstTrustedKey checks the package signature against an explicitly
// trusted public key supplied by the importer.
func verifyAgainstTrustedKey(p ExperiencePackage, trusted []byte) bool {
	if !p.IsSigned() {
		return false
	}
	pub, err := parsePublicKey(trusted)
	if err != nil {
		return false
	}
	sig, err := hex.DecodeString(p.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, p.signablePayload(), sig)
}

// parsePrivateKey accepts raw ed25519 private keys, PKCS8 PEM, or PKCS1/PKCS8
// DER. Key material is never logged or persisted.
func parsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	raw := data
	if block, rest := pem.Decode(data); block != nil {
		if len(bytes.TrimSpace(rest)) != 0 {
			return nil, fmt.Errorf("invalid private key: trailing data after PEM block")
		}
		raw = block.Bytes
	}
	// Raw ed25519 private key (32 or 64 bytes).
	if len(raw) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(raw), nil
	}
	if len(raw) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(raw)
		if len(priv) == ed25519.PrivateKeySize {
			return priv, nil
		}
	}
	// PKCS8 DER.
	if key, err := x509.ParsePKCS8PrivateKey(raw); err == nil {
		if edPriv, ok := key.(ed25519.PrivateKey); ok {
			return edPriv, nil
		}
		return nil, fmt.Errorf("private key is not ed25519")
	}
	// PKCS1 is not supported for ed25519.
	return nil, fmt.Errorf("unsupported private key format")
}

// parsePublicKey accepts raw ed25519 public keys, PKIX PEM, or PKIX DER.
func parsePublicKey(data []byte) (ed25519.PublicKey, error) {
	raw := data
	if block, rest := pem.Decode(data); block != nil {
		if len(bytes.TrimSpace(rest)) != 0 {
			return nil, fmt.Errorf("invalid public key: trailing data after PEM block")
		}
		raw = block.Bytes
	}
	if len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	if key, err := x509.ParsePKIXPublicKey(raw); err == nil {
		if edPub, ok := key.(ed25519.PublicKey); ok {
			return edPub, nil
		}
		return nil, fmt.Errorf("public key is not ed25519")
	}
	return nil, fmt.Errorf("unsupported public key format")
}

// LoadPrivateKey reads a private key file with strict safety checks: the path
// must not be a symlink and the file must not be group/world readable.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	if info, err := os.Lstat(path); err != nil {
		return nil, err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("private key path is a symlink: %s", path)
	} else if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private key must not be group/world readable: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePrivateKey(data)
}

// LoadPublicKey reads a public key file. Symlinks are accepted for public
// keys but the content is validated.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	if info, err := os.Lstat(path); err != nil {
		return nil, err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("public key path is a symlink: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePublicKey(data)
}

// PEM helpers for CLI key generation (export --sign-key path).
func marshalPrivateKeyPEM(priv ed25519.PrivateKey) []byte {
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func marshalPublicKeyPEM(pub ed25519.PublicKey) []byte {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}
