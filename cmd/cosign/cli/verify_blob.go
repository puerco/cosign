// Copyright 2021 The Rekor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"github.com/sigstore/cosign/pkg/cosign"
	"github.com/sigstore/cosign/pkg/cosign/fulcio"
	"github.com/sigstore/cosign/pkg/cosign/kms"
	"github.com/sigstore/rekor/cmd/cli/app"
)

func VerifyBlob() *ffcli.Command {
	var (
		flagset   = flag.NewFlagSet("cosign verify-blob", flag.ExitOnError)
		key       = flagset.String("key", "", "path to the public key")
		kmsVal    = flagset.String("kms", "", "verify via a public key stored in a KMS")
		cert      = flagset.String("cert", "", "path to the public certificate")
		signature = flagset.String("signature", "", "path to the signature")
	)
	return &ffcli.Command{
		Name:       "verify-blob",
		ShortUsage: "cosign verify-blob -key <key>|-cert <cert>|-kms <kms> -signature <sig> <blob>",
		ShortHelp:  "Verify a signature on the supplied blob",
		LongHelp: `Verify a signature on the supplied blob input using the specified key reference.
You may specify either a key, a certificate or a kms reference to verify against.
	If you use a key or a certificate, you must specify the path to them on disk.

The signature may be specified as a path to a file or a base64 encoded string.
The blob may be specified as a path to a file or - for stdin.

EXAMPLES
	# Verify a simple blob and message
	cosign verify-blob -key cosign.pub -signature sig msg

	# Verify a signature from an environment variable
	cosign verify-blob -key cosign.pub -signature $sig msg

	# Verify a signature against a payload from another process using process redirection
	cosign verify-blob -key cosign.pub -signature $sig <(git rev-parse HEAD)

	# Verify a signature against a KMS reference
	cosign verify-blob -kms gcpkms://projects/<PROJECT ID>/locations/<LOCATION>/keyRings/<KEYRING>/cryptoKeys/<KEY> -signature $sig <blob>`,
		FlagSet: flagset,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				return flag.ErrHelp
			}
			if err := VerifyBlobCmd(ctx, *key, *kmsVal, *cert, *signature, args[0]); err != nil {
				return errors.Wrapf(err, "verifying blob %s", args)
			}
			return nil
		},
	}
}

func isb64(data []byte) bool {
	_, err := base64.StdEncoding.DecodeString(string(data))
	return err == nil
}

func VerifyBlobCmd(ctx context.Context, keyRef, kmsVal, certRef, sigRef, blobRef string) error {
	var pubKey cosign.PublicKey
	var err error
	var cert *x509.Certificate
	switch {
	case keyRef != "":
		pubKey, err = cosign.LoadPublicKey(ctx, keyRef)
		if err != nil {
			return err
		}
	case kmsVal != "":
		pubKey, err = kms.Get(ctx, kmsVal)
		if err != nil {
			return errors.Wrap(err, "getting kms")
		}
	case certRef != "": // KEYLESS MODE!
		pems, err := ioutil.ReadFile(certRef)
		if err != nil {
			return err
		}

		certs, err := cosign.LoadCerts(string(pems))
		if err != nil {
			return err
		}
		if len(certs) == 0 {
			return errors.New("no certs found in pem file")
		}
		cert = certs[0]
		pubKey = &cosign.ECDSAPublicKey{
			Key: cert.PublicKey.(*ecdsa.PublicKey),
		}
	default:
		return errors.New("one of -key and -cert required")
	}

	var b64sig string
	// This can be the base64-encoded bytes or a path to the signature
	if _, err = os.Stat(sigRef); err != nil {
		if os.IsNotExist(err) {
			b64sig = sigRef
		} else {
			return err
		}
	} else {
		b, err := ioutil.ReadFile(filepath.Clean(sigRef))
		if err != nil {
			return err
		}
		// If in a file, it could be raw or base64-encoded.
		// We want them to be encoded eventually, but not double encoded!
		if isb64(b) {
			b64sig = string(b)
		} else {
			b64sig = base64.StdEncoding.EncodeToString(b)
		}
	}

	var blobBytes []byte
	if blobRef == "-" {
		blobBytes, err = ioutil.ReadAll(os.Stdin)
	} else {
		blobBytes, err = ioutil.ReadFile(filepath.Clean(blobRef))
	}
	if err != nil {
		return err
	}

	sig, err := base64.StdEncoding.DecodeString(b64sig)
	if err != nil {
		return err
	}
	if err := pubKey.Verify(ctx, blobBytes, sig); err != nil {
		return err
	}

	if cert != nil { // cert
		if err := cosign.TrustedCert(cert, fulcio.Roots); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Certificate is trusted by Fulcio Root CA")
		fmt.Fprintln(os.Stderr, "Email:", cert.Subject.CommonName)
	}
	fmt.Fprintln(os.Stderr, "Verified OK")

	if cosign.Experimental() {
		rekorClient, err := app.GetRekorClient(cosign.TlogServer())
		if err != nil {
			return err
		}
		var pubBytes []byte
		if pubKey != nil {
			pubBytes, err = cosign.PublicKeyPem(ctx, pubKey)
			if err != nil {
				return err
			}
		}
		if cert != nil {
			pubBytes = cosign.CertToPem(cert)
		}
		index, err := cosign.FindTlogEntry(rekorClient, b64sig, blobBytes, pubBytes)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "tlog entry verified with index: ", index)
		return nil
	}

	return nil
}
