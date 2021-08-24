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
	proto "github.com/GoogleContainerTools/skaffold/proto/v3"
)

func DeployInProgress(id int) {
	deployEvent := &proto.DeployStartedEvent{
		Id:     strconv.Itoa(id),
		TaskId: fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status: InProgress,
	}
	handler.handle(deployEvent.Id, deployEvent, DeployStartedEvent)
}

func DeployFailed(id int, err error) {
	deployEvent := &proto.DeployFailedEvent{
		Id:            strconv.Itoa(id),
		TaskId:        fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status:        Failed,
		ActionableErr: sErrors.ActionableErrV3(handler.cfg, constants.Deploy, err),
	}
	handler.handle(deployEvent.Id, deployEvent, DeployFailedEvent)
}

func DeploySucceeded(id int) {
	deployEvent := &proto.DeploySucceededEvent{
		Id:     strconv.Itoa(id),
		TaskId: fmt.Sprintf("%s-%d", constants.Deploy, handler.iteration),
		Status: Succeeded,
	}
	handler.handle(deployEvent.Id, deployEvent, DeploySucceededEvent)
}
