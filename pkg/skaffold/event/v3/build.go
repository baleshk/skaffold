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
	"fmt"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/constants"
	sErrors "github.com/GoogleContainerTools/skaffold/pkg/skaffold/errors"
	protoV3 "github.com/GoogleContainerTools/skaffold/proto/v3"
)

const (
	Cache = "Cache"
	Build = "Build"
)

func CacheCheckInProgress(artifact string) {
	buildEvent := &protoV3.BuildStartedEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Cache,
		Status:        InProgress,
		ActionableErr: nil,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildStartedEvent,
		EventType: &protoV3.EventContainer_BuildStartedEvent{
			BuildStartedEvent: buildEvent,
		}})
}

func CacheCheckMiss(artifact string) {
	buildEvent := &protoV3.BuildFailedEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Cache,
		Status:        Failed,
		ActionableErr: nil,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildFailedEvent,
		EventType: &protoV3.EventContainer_BuildFailedEvent{
			BuildFailedEvent: buildEvent,
		},
	})
}

func CacheCheckHit(artifact string) {
	buildEvent := &protoV3.BuildSucceededEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Cache,
		Status:        Succeeded,
		ActionableErr: nil,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildSucceededEvent,
		EventType: &protoV3.EventContainer_BuildSucceededEvent{
			BuildSucceededEvent: buildEvent,
		}})
}

func BuildInProgress(artifact string) {
	buildEvent := &protoV3.BuildStartedEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Build,
		Status:        InProgress,
		ActionableErr: nil,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildStartedEvent,
		EventType: &protoV3.EventContainer_BuildStartedEvent{
			BuildStartedEvent: buildEvent,
		}})
}

func BuildFailed(artifact string, err error) {
	var aErr *protoV3.ActionableErr
	if err != nil {
		aErr = sErrors.ActionableErrV3(handler.cfg, constants.Build, err)
	}
	buildEvent := &protoV3.BuildFailedEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Build,
		Status:        Failed,
		ActionableErr: aErr,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildFailedEvent,
		EventType: &protoV3.EventContainer_BuildFailedEvent{
			BuildFailedEvent: buildEvent,
		}})
}

func BuildSucceeded(artifact string) {
	buildEvent := &protoV3.BuildSucceededEvent{
		Id:            artifact,
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Build,
		Status:        Succeeded,
		ActionableErr: nil,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildSucceededEvent,
		EventType: &protoV3.EventContainer_BuildSucceededEvent{
			BuildSucceededEvent: buildEvent,
		}})
}

func BuildCanceled(artifact string, err error) {
	var aErr *protoV3.ActionableErr
	if err != nil {
		aErr = sErrors.ActionableErrV3(handler.cfg, constants.Build, err)
	}
	buildEvent := &protoV3.BuildCancelledEvent{
		TaskId:        fmt.Sprintf("%s-%d", constants.Build, handler.iteration),
		Artifact:      artifact,
		Step:          Build,
		Status:        Canceled,
		ActionableErr: aErr,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: BuildCancelledEvent,
		EventType: &protoV3.EventContainer_BuildCancelledEvent{
			BuildCancelledEvent: buildEvent,
		}})
}
