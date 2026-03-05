/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package grpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusPassOrWrapError(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		err := (&Server{}).statusPassOrWrapError(nil, codes.Internal, "internal error")
		assert.Nil(t, err)
	})

	t.Run("non-status-error", func(t *testing.T) {
		terr := errors.New("test-error")

		err := (&Server{}).statusPassOrWrapError(terr, codes.Internal, "internal error: %v", terr)
		assert.NotNil(t, err)
		assert.NotEqual(t, terr, err)

		se, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, se.Code())
		assert.Contains(t, se.Err().Error(), terr.Error())
	})

	t.Run("status-error", func(t *testing.T) {
		terr := status.Errorf(codes.Canceled, "cancelled operation")

		err := (&Server{}).statusPassOrWrapError(terr, codes.Internal, "internal error: %v", terr)
		assert.NotNil(t, err)
		assert.Equal(t, terr, err)

		se, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Canceled, se.Code())
		assert.Equal(t, se.Err().Error(), terr.Error())
	})
}
