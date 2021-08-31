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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	//nolint:golint,staticcheck
	"github.com/golang/protobuf/jsonpb"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/constants"
	sErrors "github.com/GoogleContainerTools/skaffold/pkg/skaffold/errors"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/schema/util"
	protoV3 "github.com/GoogleContainerTools/skaffold/proto/v3"
	proto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	NotStarted = "NotStarted"
	InProgress = "InProgress"
	Complete   = "Complete"
	Failed     = "Failed"
	Info       = "Information"
	Started    = "Started"
	Succeeded  = "Succeeded"
	Terminated = "Terminated"
	Canceled   = "Canceled"

	SubtaskIDNone = "-1"
)

const (
	BuildStartedEvent                 string = "BuildStartedEvent"
	BuildFailedEvent                  string = "BuildFailedEvent"
	BuildSucceededEvent               string = "BuildSucceededEvent"
	BuildCancelledEvent               string = "BuildCancelledEvent"
	DebuggingContainerStartedEvent    string = "DebuggingContainerStartedEvent"
	DebuggingContainerTerminatedEvent string = "DebuggingContainerTerminatedEvent"
	DeployStartedEvent                string = "DeployStartedEvent"
	DeployFailedEvent                 string = "DeployFailedEvent"
	DeploySucceededEvent              string = "DeploySucceededEvent"
	SkaffoldLogEvent                  string = "SkaffoldLogEvent"
	MetaEvent                         string = "MetaEvent"
	RenderStartedEvent                string = "RenderStartedEvent"
	RenderFailedEvent                 string = "RenderFailedEvent"
	RenderSucceededEvent              string = "RenderSucceededEvent"
	StatusCheckFailedEvent            string = "StatusCheckFailedEvent"
	StatusCheckStartedEvent           string = "StatusCheckStartedEvent"
	StatusCheckSucceededEvent         string = "StatusCheckSucceededEvent"
	TestFailedEvent                   string = "TestFailedEvent"
	TestSucceededEvent                string = "TestSucceededEvent"
	TestStartedEvent                  string = "TestStartedEvent"
	ApplicationLogEvent               string = "ApplicationLogEvent"
	PortForwardedEvent                string = "PortForwardedEvent"
	FileSyncEvent                     string = "FileSyncEvent"
	TaskStartedEvent                  string = "TaskStartedEvent"
	TaskFailedEvent                   string = "TaskFailedEvent"
	TaskCompletedEvent                string = "TaskCompletedEvent"
)

var handler = newHandler()

func newHandler() *eventHandler {
	h := &eventHandler{
		eventChan: make(chan *protoV3.EventContainer),
	}
	go func() {
		for {
			ev, open := <-h.eventChan
			if !open {
				break
			}
			h.handleExec(ev)
		}
	}()
	return h
}

type eventHandler struct {
	eventLog                []protoV3.Event
	logLock                 sync.Mutex
	applicationLogs         []protoV3.Event
	applicationLogsLock     sync.Mutex
	cfg                     Config
	iteration               int
	state                   protoV3.State
	stateLock               sync.Mutex
	eventChan               chan *protoV3.EventContainer
	eventListeners          []*listener
	applicationLogListeners []*listener
}

type listener struct {
	callback func(*protoV3.Event) error
	errors   chan error
	closed   bool
}

func GetIteration() int {
	return handler.iteration
}

func GetState() (*protoV3.State, error) {
	state := handler.getState()
	return &state, nil
}

func ForEachEvent(callback func(*protoV3.Event) error) error {
	return handler.forEachEvent(callback)
}

func ForEachApplicationLog(callback func(*protoV3.Event) error) error {
	return handler.forEachApplicationLog(callback)
}

func Handle(event *protoV3.Event) error {
	if event != nil {
		eventContainer := handler.getEventContainer(event)
		eventContainer.Id = event.Id
		eventContainer.Source = event.Source
		eventContainer.Type = event.Type
		eventContainer.Time = event.Time
		handler.handle(eventContainer)
	}
	return nil
}

func (ev *eventHandler) getState() protoV3.State {
	ev.stateLock.Lock()
	// Deep copy
	buf, _ := json.Marshal(ev.state)
	ev.stateLock.Unlock()

	var state protoV3.State
	json.Unmarshal(buf, &state)

	return state
}

func (ev *eventHandler) log(event *protoV3.Event, listeners *[]*listener, log *[]protoV3.Event, lock sync.Locker) {
	lock.Lock()

	for _, listener := range *listeners {
		if listener.closed {
			continue
		}

		if err := listener.callback(event); err != nil {
			listener.errors <- err
			listener.closed = true
		}
	}
	*log = append(*log, *event)

	lock.Unlock()
}

func (ev *eventHandler) logEvent(eventContainer *protoV3.EventContainer, eventData anypb.Any) {
	event := createMainEvent(eventContainer, eventData)
	ev.log(event, &ev.eventListeners, &ev.eventLog, &ev.logLock)
}

func (ev *eventHandler) logApplicationLog(eventContainer *protoV3.EventContainer, eventData anypb.Any) {
	event := createMainEvent(eventContainer, eventData)
	ev.log(event, &ev.applicationLogListeners, &ev.applicationLogs, &ev.applicationLogsLock)
}

func createMainEvent(eventContainer *protoV3.EventContainer, eventData anypb.Any) *protoV3.Event {
	return &protoV3.Event{
		Id:              uuid.New().String(),
		Type:            eventContainer.Type,
		Data:            &eventData,
		Specversion:     "1.0",
		Source:          "skaffold.dev",
		Time:            eventContainer.Time,
		Datacontenttype: "application/protobuf"}
}
func (ev *eventHandler) forEach(listeners *[]*listener, log *[]protoV3.Event, lock sync.Locker, callback func(*protoV3.Event) error) error {
	listener := &listener{
		callback: callback,
		errors:   make(chan error),
	}

	lock.Lock()

	oldEvents := make([]protoV3.Event, len(*log))
	copy(oldEvents, *log)
	*listeners = append(*listeners, listener)

	lock.Unlock()

	for i := range oldEvents {
		if err := callback(&oldEvents[i]); err != nil {
			// listener should maybe be closed
			return err
		}
	}

	return <-listener.errors
}

func (ev *eventHandler) forEachEvent(callback func(*protoV3.Event) error) error {
	return ev.forEach(&ev.eventListeners, &ev.eventLog, &ev.logLock, callback)
}

func (ev *eventHandler) forEachApplicationLog(callback func(*protoV3.Event) error) error {
	return ev.forEach(&ev.applicationLogListeners, &ev.applicationLogs, &ev.applicationLogsLock, callback)
}

func emptyState(cfg Config) protoV3.State {
	builds := map[string]string{}
	for _, p := range cfg.GetPipelines() {
		for _, a := range p.Build.Artifacts {
			builds[a.ImageName] = NotStarted
		}
	}
	metadata := initializeMetadata(cfg.GetPipelines(), cfg.GetKubeContext(), cfg.GetRunID())
	return emptyStateWithArtifacts(builds, metadata, cfg.AutoBuild(), cfg.AutoDeploy(), cfg.AutoSync())
}

func emptyStateWithArtifacts(builds map[string]string, metadata *protoV3.Metadata, autoBuild, autoDeploy, autoSync bool) protoV3.State {
	return protoV3.State{
		BuildState: &protoV3.BuildState{
			Artifacts:   builds,
			AutoTrigger: autoBuild,
			StatusCode:  protoV3.StatusCode_OK,
		},
		TestState: &protoV3.TestState{
			Status:     NotStarted,
			StatusCode: protoV3.StatusCode_OK,
		},
		RenderState: &protoV3.RenderState{
			Status:     NotStarted,
			StatusCode: protoV3.StatusCode_OK,
		},
		DeployState: &protoV3.DeployState{
			Status:      NotStarted,
			AutoTrigger: autoDeploy,
			StatusCode:  protoV3.StatusCode_OK,
		},
		StatusCheckState: emptyStatusCheckState(),
		ForwardedPorts:   make(map[int32]*protoV3.PortForwardEvent),
		FileSyncState: &protoV3.FileSyncState{
			Status:      NotStarted,
			AutoTrigger: autoSync,
		},
		Metadata: metadata,
	}
}

// ResetStateOnBuild resets the build, test, deploy and sync state
func ResetStateOnBuild() {
	builds := map[string]string{}
	for k := range handler.getState().BuildState.Artifacts {
		builds[k] = NotStarted
	}
	autoBuild, autoDeploy, autoSync := handler.getState().BuildState.AutoTrigger, handler.getState().DeployState.AutoTrigger, handler.getState().FileSyncState.AutoTrigger
	newState := emptyStateWithArtifacts(builds, handler.getState().Metadata, autoBuild, autoDeploy, autoSync)
	handler.setState(newState)
}

// ResetStateOnTest resets the test, deploy, sync and status check state
func ResetStateOnTest() {
	newState := handler.getState()
	newState.TestState.Status = NotStarted
	handler.setState(newState)
}

// ResetStateOnDeploy resets the deploy, sync and status check state
func ResetStateOnDeploy() {
	newState := handler.getState()
	newState.DeployState.Status = NotStarted
	newState.DeployState.StatusCode = protoV3.StatusCode_OK
	newState.StatusCheckState = emptyStatusCheckState()
	newState.ForwardedPorts = map[int32]*protoV3.PortForwardEvent{}
	newState.DebuggingContainers = nil
	handler.setState(newState)
}

func UpdateStateAutoBuildTrigger(t bool) {
	newState := handler.getState()
	newState.BuildState.AutoTrigger = t
	handler.setState(newState)
}

func UpdateStateAutoDeployTrigger(t bool) {
	newState := handler.getState()
	newState.DeployState.AutoTrigger = t
	handler.setState(newState)
}

func UpdateStateAutoSyncTrigger(t bool) {
	newState := handler.getState()
	newState.FileSyncState.AutoTrigger = t
	handler.setState(newState)
}

func emptyStatusCheckState() *protoV3.StatusCheckState {
	return &protoV3.StatusCheckState{
		Status:     NotStarted,
		Resources:  map[string]string{},
		StatusCode: protoV3.StatusCode_OK,
	}
}

// InitializeState instantiates the global state of the skaffold runner, as well as the event log.
func InitializeState(cfg Config) {
	handler.cfg = cfg
	handler.setState(emptyState(cfg))
}

func AutoTriggerDiff(phase constants.Phase, val bool) (bool, error) {
	switch phase {
	case constants.Build:
		return val != handler.getState().BuildState.AutoTrigger, nil
	case constants.Sync:
		return val != handler.getState().FileSyncState.AutoTrigger, nil
	case constants.Deploy:
		return val != handler.getState().DeployState.AutoTrigger, nil
	default:
		return false, fmt.Errorf("unknown Phase %v not found in handler state", phase)
	}
}

func TaskInProgress(task constants.Phase, description string) {
	// Special casing to increment iteration and clear application and skaffold logs
	if task == constants.DevLoop {
		handler.iteration++

		handler.applicationLogs = []protoV3.Event{}
	}

	event := &protoV3.TaskStartedEvent{
		Id:          fmt.Sprintf("%s-%d", task, handler.iteration),
		Task:        string(task),
		Description: description,
		Iteration:   int32(handler.iteration),
		Status:      InProgress,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: TaskStartedEvent,
		EventType: &protoV3.EventContainer_TaskStartedEvent{
			TaskStartedEvent: event,
		}})

}

func TaskFailed(task constants.Phase, err error) {
	ae := sErrors.ActionableErrV3(handler.cfg, task, err)
	event := &protoV3.TaskFailedEvent{
		Id:            fmt.Sprintf("%s-%d", task, handler.iteration),
		Task:          string(task),
		Iteration:     int32(handler.iteration),
		Status:        Failed,
		ActionableErr: ae,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: TaskFailedEvent,
		EventType: &protoV3.EventContainer_TaskFailedEvent{
			TaskFailedEvent: event,
		}})
}

func TaskSucceeded(task constants.Phase) {
	event := &protoV3.TaskCompletedEvent{
		Id:        fmt.Sprintf("%s-%d", task, handler.iteration),
		Task:      string(task),
		Iteration: int32(handler.iteration),
		Status:    Succeeded,
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: TaskCompletedEvent,
		EventType: &protoV3.EventContainer_TaskCompletedEvent{
			TaskCompletedEvent: event,
		}})
}

// PortForwarded notifies that a remote port has been forwarded locally.
func PortForwarded(localPort int32, remotePort util.IntOrString, podName, containerName, namespace string, portName string, resourceType, resourceName, address string) {
	event := &protoV3.PortForwardEvent{
		TaskId:        fmt.Sprintf("%s-%d", constants.PortForward, handler.iteration),
		LocalPort:     localPort,
		PodName:       podName,
		ContainerName: containerName,
		Namespace:     namespace,
		PortName:      portName,
		ResourceType:  resourceType,
		ResourceName:  resourceName,
		Address:       address,
		TargetPort: &protoV3.IntOrString{
			Type:   int32(remotePort.Type),
			IntVal: int32(remotePort.IntVal),
			StrVal: remotePort.StrVal,
		},
	}
	handler.handleInternal(&protoV3.EventContainer{
		Type: PortForwardedEvent,
		EventType: &protoV3.EventContainer_PortEvent{
			PortEvent: event,
		}})
}

func (ev *eventHandler) setState(state protoV3.State) {
	ev.stateLock.Lock()
	ev.state = state
	ev.stateLock.Unlock()
}

func (ev *eventHandler) handleInternal(event *protoV3.EventContainer) {
	event.Id = uuid.New().String()
	event.Source = "skaffold.dev"
	event.Time = timestamppb.Now()
	ev.handle(event)
}

func (ev *eventHandler) handle(event *protoV3.EventContainer) {
	ev.eventChan <- event

	if _, ok := event.GetEventType().(*protoV3.EventContainer_TerminationEvent); ok {
		// close the event channel indicating there are no more events to all the
		// receivers
		close(ev.eventChan)
	}
}

func (ev *eventHandler) handleExec(event *protoV3.EventContainer) {
	protoEvent := &anypb.Any{}
	switch e := event.GetEventType().(type) {
	case *protoV3.EventContainer_ApplicationLogEvent:
		appEvent := e.ApplicationLogEvent
		anypb.MarshalFrom(protoEvent, appEvent, proto.MarshalOptions{})
		ev.logApplicationLog(event, *protoEvent)
		return
	case *protoV3.EventContainer_BuildSucceededEvent:
		buildEvent := e.BuildSucceededEvent
		anypb.MarshalFrom(protoEvent, buildEvent, proto.MarshalOptions{})
		if buildEvent.Step == Build {
			ev.stateLock.Lock()
			ev.state.BuildState.Artifacts[buildEvent.Artifact] = buildEvent.Status
			ev.stateLock.Unlock()
		}
	case *protoV3.EventContainer_BuildStartedEvent:
		buildEvent := e.BuildStartedEvent
		anypb.MarshalFrom(protoEvent, buildEvent, proto.MarshalOptions{})
		if buildEvent.Step == Build {
			ev.stateLock.Lock()
			ev.state.BuildState.Artifacts[buildEvent.Artifact] = buildEvent.Status
			ev.stateLock.Unlock()
		}
	case *protoV3.EventContainer_BuildFailedEvent:
		buildEvent := e.BuildFailedEvent
		anypb.MarshalFrom(protoEvent, buildEvent, proto.MarshalOptions{})
		if buildEvent.Step == Build {
			ev.stateLock.Lock()
			ev.state.BuildState.Artifacts[buildEvent.Artifact] = buildEvent.Status
			ev.stateLock.Unlock()
		}
	case *protoV3.EventContainer_BuildCancelledEvent:
		buildEvent := e.BuildCancelledEvent
		anypb.MarshalFrom(protoEvent, buildEvent, proto.MarshalOptions{})
		if buildEvent.Step == Build {
			ev.stateLock.Lock()
			ev.state.BuildState.Artifacts[buildEvent.Artifact] = buildEvent.Status
			ev.stateLock.Unlock()
		}
	case *protoV3.EventContainer_TestFailedEvent:
		te := e.TestFailedEvent
		anypb.MarshalFrom(protoEvent, te, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.TestState.Status = te.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_TestStartedEvent:
		te := e.TestStartedEvent
		anypb.MarshalFrom(protoEvent, te, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.TestState.Status = te.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_TestSucceededEvent:
		te := e.TestSucceededEvent
		anypb.MarshalFrom(protoEvent, te, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.TestState.Status = te.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_RenderFailedEvent:
		re := e.RenderFailedEvent
		anypb.MarshalFrom(protoEvent, re, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.RenderState.Status = re.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_RenderSucceededEvent:
		re := e.RenderSucceededEvent
		anypb.MarshalFrom(protoEvent, re, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.RenderState.Status = re.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_RenderStartedEvent:
		re := e.RenderStartedEvent
		anypb.MarshalFrom(protoEvent, re, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.RenderState.Status = re.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_DeployStartedEvent:
		de := e.DeployStartedEvent
		anypb.MarshalFrom(protoEvent, de, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.DeployState.Status = de.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_DeployFailedEvent:
		de := e.DeployFailedEvent
		anypb.MarshalFrom(protoEvent, de, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.DeployState.Status = de.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_DeploySucceededEvent:
		de := e.DeploySucceededEvent
		anypb.MarshalFrom(protoEvent, de, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.DeployState.Status = de.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_PortEvent:
		pe := e.PortEvent
		anypb.MarshalFrom(protoEvent, pe, proto.MarshalOptions{})
		ev.stateLock.Lock()
		if ev.state.ForwardedPorts == nil {
			ev.state.ForwardedPorts = map[int32]*protoV3.PortForwardEvent{}
		}
		ev.state.ForwardedPorts[pe.LocalPort] = pe
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_StatusCheckStartedEvent:
		se := e.StatusCheckStartedEvent
		anypb.MarshalFrom(protoEvent, se, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.StatusCheckState.Resources[se.Resource] = se.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_StatusCheckSucceededEvent:
		se := e.StatusCheckSucceededEvent
		anypb.MarshalFrom(protoEvent, se, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.StatusCheckState.Resources[se.Resource] = se.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_StatusCheckFailedEvent:
		se := e.StatusCheckFailedEvent
		anypb.MarshalFrom(protoEvent, se, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.StatusCheckState.Resources[se.Resource] = se.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_FileSyncEvent:
		fse := e.FileSyncEvent
		anypb.MarshalFrom(protoEvent, fse, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.FileSyncState.Status = fse.Status
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_DebuggingContainerStartedEvent:
		de := e.DebuggingContainerStartedEvent
		anypb.MarshalFrom(protoEvent, de, proto.MarshalOptions{})
		ev.stateLock.Lock()
		ev.state.DebuggingContainers = append(ev.state.DebuggingContainers, &protoV3.DebuggingContainerState{
			Id:            de.Id,
			TaskId:        de.TaskId,
			Status:        de.Status,
			PodName:       de.PodName,
			ContainerName: de.ContainerName,
			Namespace:     de.Namespace,
			Artifact:      de.Artifact,
			Runtime:       de.Runtime,
			WorkingDir:    de.WorkingDir,
			DebugPorts:    de.DebugPorts,
		})
		ev.stateLock.Unlock()
	case *protoV3.EventContainer_DebuggingContainerTerminatedEvent:
		de := e.DebuggingContainerTerminatedEvent
		anypb.MarshalFrom(protoEvent, de, proto.MarshalOptions{})
		ev.stateLock.Lock()
		n := 0
		for _, x := range ev.state.DebuggingContainers {
			if x.Namespace != de.Namespace || x.PodName != de.PodName || x.ContainerName != de.ContainerName {
				ev.state.DebuggingContainers[n] = x
				n++
			}
		}

		ev.state.DebuggingContainers = ev.state.DebuggingContainers[:n]
		ev.stateLock.Unlock()

	case *protoV3.EventContainer_SkaffoldLogEvent:
		logEvent := e.SkaffoldLogEvent
		anypb.MarshalFrom(protoEvent, logEvent, proto.MarshalOptions{})

	case *protoV3.EventContainer_MetaEvent:
		metaEvent := e.MetaEvent
		anypb.MarshalFrom(protoEvent, metaEvent, proto.MarshalOptions{})

	case *protoV3.EventContainer_TaskStartedEvent:
		metaEvent := e.TaskStartedEvent
		anypb.MarshalFrom(protoEvent, metaEvent, proto.MarshalOptions{})

	case *protoV3.EventContainer_TaskFailedEvent:
		metaEvent := e.TaskFailedEvent
		anypb.MarshalFrom(protoEvent, metaEvent, proto.MarshalOptions{})

	case *protoV3.EventContainer_TaskCompletedEvent:
		metaEvent := e.TaskCompletedEvent
		anypb.MarshalFrom(protoEvent, metaEvent, proto.MarshalOptions{})

	}

	ev.logEvent(event, *protoEvent)
}

// SaveEventsToFile saves the current event log to the filepath provided
func SaveEventsToFile(fp string) error {
	handler.logLock.Lock()
	f, err := os.OpenFile(fp, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("opening %s: %w", fp, err)
	}
	defer f.Close()
	marshaller := jsonpb.Marshaler{}
	for _, ev := range handler.eventLog {
		contents := bytes.NewBuffer([]byte{})
		if err := marshaller.Marshal(contents, &ev); err != nil {
			return fmt.Errorf("marshalling event: %w", err)
		}
		if _, err := f.WriteString(contents.String() + "\n"); err != nil {
			return fmt.Errorf("writing string: %w", err)
		}
	}
	handler.logLock.Unlock()
	return nil
}

func (ev *eventHandler) getEventContainer(event *protoV3.Event) *protoV3.EventContainer {
	switch event.Type {
	case ApplicationLogEvent:
		protoEvent := &protoV3.ApplicationLogEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})

		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_ApplicationLogEvent{
				ApplicationLogEvent: protoEvent,
			}}
	case BuildSucceededEvent:
		protoEvent := &protoV3.BuildSucceededEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_BuildSucceededEvent{
				BuildSucceededEvent: protoEvent,
			}}
	case BuildStartedEvent:
		protoEvent := &protoV3.BuildStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_BuildStartedEvent{
				BuildStartedEvent: protoEvent,
			}}

	case BuildFailedEvent:
		protoEvent := &protoV3.BuildFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_BuildFailedEvent{
				BuildFailedEvent: protoEvent,
			}}
	case BuildCancelledEvent:
		protoEvent := &protoV3.BuildCancelledEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_BuildCancelledEvent{
				BuildCancelledEvent: protoEvent,
			}}
	case TestFailedEvent:
		protoEvent := &protoV3.TestFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_TestFailedEvent{
				TestFailedEvent: protoEvent,
			}}
	case TestStartedEvent:
		protoEvent := &protoV3.TestStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_TestStartedEvent{
				TestStartedEvent: protoEvent,
			}}
	case TestSucceededEvent:
		protoEvent := &protoV3.TestSucceededEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_TestSucceededEvent{
				TestSucceededEvent: protoEvent,
			}}
	case RenderFailedEvent:
		protoEvent := &protoV3.RenderFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_RenderFailedEvent{
				RenderFailedEvent: protoEvent,
			}}
	case RenderSucceededEvent:
		protoEvent := &protoV3.RenderSucceededEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_RenderSucceededEvent{
				RenderSucceededEvent: protoEvent,
			}}
	case RenderStartedEvent:
		protoEvent := &protoV3.RenderStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_RenderStartedEvent{
				RenderStartedEvent: protoEvent,
			}}
	case DeployStartedEvent:
		protoEvent := &protoV3.DeployStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_DeployStartedEvent{
				DeployStartedEvent: protoEvent,
			}}
	case DeployFailedEvent:
		protoEvent := &protoV3.DeployFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_DeployFailedEvent{
				DeployFailedEvent: protoEvent,
			}}
	case DeploySucceededEvent:
		protoEvent := &protoV3.DeploySucceededEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_DeploySucceededEvent{
				DeploySucceededEvent: protoEvent,
			}}
	case PortForwardedEvent:
		protoEvent := &protoV3.PortForwardEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_PortEvent{
				PortEvent: protoEvent,
			}}
	case StatusCheckStartedEvent:
		protoEvent := &protoV3.StatusCheckStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_StatusCheckStartedEvent{
				StatusCheckStartedEvent: protoEvent,
			}}
	case StatusCheckSucceededEvent:
		protoEvent := &protoV3.StatusCheckSucceededEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_StatusCheckSucceededEvent{
				StatusCheckSucceededEvent: protoEvent,
			}}
	case StatusCheckFailedEvent:
		protoEvent := &protoV3.StatusCheckFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_StatusCheckFailedEvent{
				StatusCheckFailedEvent: protoEvent,
			}}
	case FileSyncEvent:
		protoEvent := &protoV3.FileSyncEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_FileSyncEvent{
				FileSyncEvent: protoEvent,
			}}
	case DebuggingContainerStartedEvent:
		protoEvent := &protoV3.DebuggingContainerStartedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_DebuggingContainerStartedEvent{
				DebuggingContainerStartedEvent: protoEvent,
			}}
	case DebuggingContainerTerminatedEvent:
		protoEvent := &protoV3.DebuggingContainerTerminatedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_DebuggingContainerTerminatedEvent{
				DebuggingContainerTerminatedEvent: protoEvent,
			}}
	case SkaffoldLogEvent:
		protoEvent := &protoV3.SkaffoldLogEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_SkaffoldLogEvent{
				SkaffoldLogEvent: protoEvent,
			}}
	case MetaEvent:
		protoEvent := &protoV3.MetaEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_MetaEvent{
				MetaEvent: protoEvent,
			}}
	case TaskCompletedEvent:
		protoEvent := &protoV3.TaskCompletedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_TaskCompletedEvent{
				TaskCompletedEvent: protoEvent,
			}}
	case TaskFailedEvent:
		protoEvent := &protoV3.TaskFailedEvent{}
		anypb.UnmarshalTo(event.Data, protoEvent, proto.UnmarshalOptions{})
		return &protoV3.EventContainer{
			EventType: &protoV3.EventContainer_TaskFailedEvent{
				TaskFailedEvent: protoEvent,
			}}

	}
	return nil
}
