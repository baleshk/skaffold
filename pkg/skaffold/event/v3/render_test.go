/*
Copyright 2021 The Skaffold Authors

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

package v3

import (
	"errors"
	"testing"

	latestV1 "github.com/GoogleContainerTools/skaffold/pkg/skaffold/schema/latest/v1"
)

func TestHandleRenderSubtaskEvent(t *testing.T) {
	t.Run("In Progress", func(t *testing.T) {
		defer func() { handler = newHandler() }()
		handler.state = emptyState(mockCfg([]latestV1.Pipeline{{}}, "test"))
		wait(t, func() bool { return handler.getState().RenderState.Status == NotStarted })
		RendererInProgress(1)
		wait(t, func() bool { return handler.getState().RenderState.Status == InProgress })
	})

	t.Run("Failed", func(t *testing.T) {
		defer func() { handler = newHandler() }()
		handler.state = emptyState(mockCfg([]latestV1.Pipeline{{}}, "test"))
		wait(t, func() bool { return handler.getState().RenderState.Status == NotStarted })
		RendererFailed(1, errors.New("Render failed"))
		wait(t, func() bool { return handler.getState().RenderState.Status == Failed })
	})

	t.Run("Succeeded", func(t *testing.T) {
		defer func() { handler = newHandler() }()
		handler.state = emptyState(mockCfg([]latestV1.Pipeline{{}}, "test"))
		wait(t, func() bool { return handler.getState().RenderState.Status == NotStarted })
		RendererSucceeded(1)
		wait(t, func() bool { return handler.getState().RenderState.Status == Succeeded })
	})
}
