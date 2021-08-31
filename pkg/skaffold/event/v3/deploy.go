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
	"strconv"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/constants"
	sErrors "github.com/GoogleContainerTools/skaffold/pkg/skaffold/errors"
	protoV3 "github.com/GoogleContainerTools/skaffold/proto/v3"
)

func DeployInProgress(id int) {
	deployEvent := &protoV3.DeployStartedEvent{
		Id:     strconv.Itoa(id),
		TaskId: fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status: InProgress,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: DeployStartedEvent,
		EventType: &protoV3.EventContainer_DeployStartedEvent{
			DeployStartedEvent: deployEvent,
		}})
}

func DeployFailed(id int, err error) {
	deployEvent := &protoV3.DeployFailedEvent{
		Id:            strconv.Itoa(id),
		TaskId:        fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status:        Failed,
		ActionableErr: sErrors.ActionableErrV3(handler.cfg, constants.Deploy, err),
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: DeployFailedEvent,
		EventType: &protoV3.EventContainer_DeployFailedEvent{
			DeployFailedEvent: deployEvent,
		}})
}

func DeploySucceeded(id int) {
	deployEvent := &protoV3.DeploySucceededEvent{
		Id:     strconv.Itoa(id),
		TaskId: fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status: Succeeded,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: DeploySucceededEvent,
		EventType: &protoV3.EventContainer_DeploySucceededEvent{
			DeploySucceededEvent: deployEvent,
		}})
}
