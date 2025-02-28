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
	"errors"
	"testing"
)

// TestSignCmdLocalKeyAndKms verifies the SignCmd returns an error
// if both a local key path and a KMS key path are specified
func TestSignCmdLocalKeyAndKms(t *testing.T) {
	ctx := context.Background()

	// specify both local and KMS keys
	keyPath := "testLocalPath"
	kmsVal := "testKmsVal"

	err := SignCmd(ctx, keyPath, "", false, "", map[string]string{}, kmsVal, GetPass, false)

	if (errors.Is(err, &KeyParseError{}) == false) {
		t.Fatal("expected KeyParseError")
	}
}
