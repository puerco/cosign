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

package cosign

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	"github.com/theupdateframework/go-tuf/encrypted"
)

const (
	pemType  = "ENCRYPTED COSIGN PRIVATE KEY"
	sigkey   = "dev.cosignproject.cosign/signature"
	certkey  = "dev.sigstore.cosign/certificate"
	chainkey = "dev.sigstore.cosign/chain"
)

func LoadPrivateKey(key []byte, pass []byte) (*ECDSAKey, error) {
	// Decrypt first
	p, _ := pem.Decode(key)
	if p == nil {
		return nil, errors.New("invalid pem block")
	}
	if p.Type != pemType {
		return nil, fmt.Errorf("unsupported pem type: %s", p.Type)
	}

	x509Encoded, err := encrypted.Decrypt(p.Bytes, pass)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	pk, err := x509.ParsePKCS8PrivateKey(x509Encoded)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}
	epk, ok := pk.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid private key")
	}
	return WithECDSAKey(epk), nil
}

type SimpleSigning struct {
	Critical Critical
	Optional map[string]string
}

type Critical struct {
	Identity Identity
	Image    Image
	Type     string
}

type Identity struct {
	DockerReference string `json:"docker-reference"`
}

type Image struct {
	DockerManifestDigest string `json:"Docker-manifest-digest"`
}

type Signer interface {
	Sign(ctx context.Context, payload []byte) (signature []byte, err error)
}

func PayloadSignature(ctx context.Context, signer Signer, payload []byte) (signature []byte, err error) {
	signature, err = signer.Sign(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create signature: %v", err)
	}
	return signature, nil
}

func ImageSignature(ctx context.Context, signer Signer, img v1.Descriptor, payloadAnnotations map[string]string) (payload, signature []byte, err error) {
	signable := &ImagePayload{Img: img, Annotations: payloadAnnotations}
	payload, err = signable.MarshalJSON()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create image signature payload: %v", err)
	}
	signature, err = PayloadSignature(ctx, signer, payload)
	if err != nil {
		return nil, nil, err
	}
	return payload, signature, nil
}
