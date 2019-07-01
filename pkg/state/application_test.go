/*
Copyright 2019 Cloudera, Inc.  All rights reserved.

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

package state

import (
	"github.com/cloudera/k8s-shim/pkg/scheduler/conf"
	"github.com/cloudera/scheduler-interface/lib/go/si"
	"github.com/cloudera/yunikorn-core/pkg/api"
	"gotest.tools/assert"
	"k8s.io/api/core/v1"
	apis "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestNewApplication(t *testing.T) {
	app := NewApplication("app00001", "root.queue", newMockSchedulerApi())
	assert.Equal(t, app.GetApplicationId(), "app00001" )
	assert.Equal(t, app.GetApplicationState(), States().Application.New)
	assert.Equal(t, app.partition, conf.DefaultPartition)
	assert.Equal(t, len(app.taskMap), 0)
	assert.Equal(t, app.GetApplicationState(), States().Application.New)
	assert.Equal(t, app.queue, "root.queue")
}

func TestSubmitApplication(t *testing.T) {
	app := NewApplication("app00001", "root.abc", newMockSchedulerApi())

	app.handle(NewSubmitApplicationEvent(app.applicationId))
	assertAppState(t, app, States().Application.Submitted, 10*time.Second)

	// app already submitted
	app.handle(NewSubmitApplicationEvent(app.applicationId))
	assertAppState(t, app, States().Application.Submitted, 10*time.Second)
}

func TestRunApplication(t *testing.T) {
	ms := &MockSchedulerApi{}
	ms.updateFn = func(request *si.UpdateRequest) error {
		assert.Equal(t, len(request.NewApplications), 1)
		assert.Equal(t, request.NewApplications[0].ApplicationId, "app00001")
		assert.Equal(t, request.NewApplications[0].QueueName, "root.abc")
		return nil
	}

	app := NewApplication("app00001", "root.abc", ms)

	// app must be submitted before being able to run
	app.handle(NewRunApplicationEvent(app.applicationId, nil))
	assertAppState(t, app, States().Application.New, 3*time.Second)

	// submit the app
	app.handle(NewSubmitApplicationEvent(app.applicationId))
	assertAppState(t, app, States().Application.Submitted, 3*time.Second)


	// app must be accepted first
	app.handle(NewRunApplicationEvent(app.applicationId, nil))
	assertAppState(t, app, States().Application.Submitted, 3*time.Second)
}

func TestGetApplicationIdFromPod(t *testing.T) {
	// defined in label
	pod := v1.Pod{
		TypeMeta: apis.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: apis.ObjectMeta{
			Name:         "pod00001",
			Namespace:    "default",
			UID:          "UID-POD-00001",
			Labels: map[string]string{
				"applicationId": "app00001",
				"queue":         "root.a",
			},
		},
		Spec:   v1.PodSpec{},
		Status: v1.PodStatus{},
	}
	appId, err := GenerateApplicationIdFromPod(&pod)
	assert.Equal(t, appId, "app00001")
	assert.Equal(t, err, nil)

	// defined in annotations
	pod = v1.Pod{
		TypeMeta: apis.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: apis.ObjectMeta{
			Name:         "pod00001",
			Namespace:    "default",
			UID:          "UID-POD-00001",
			Annotations: map[string]string{
				"applicationId": "app00002",
				"queue":         "root.a",
			},
		},
		Spec:   v1.PodSpec{},
		Status: v1.PodStatus{},
	}
	appId, err = GenerateApplicationIdFromPod(&pod)
	assert.Equal(t, appId, "app00002")
	assert.Equal(t, err, nil)

	// spark app-id
	pod = v1.Pod{
		TypeMeta: apis.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: apis.ObjectMeta{
			Name:         "pod00001",
			Namespace:    "default",
			UID:          "UID-POD-00001",
			Labels: map[string]string{
				"spark-app-id": "spark-0001",
				"queue":         "root.a",
			},
		},
		Spec:   v1.PodSpec{},
		Status: v1.PodStatus{},
	}
	appId, err = GenerateApplicationIdFromPod(&pod)
	assert.Equal(t, appId, "spark-0001")
	assert.Equal(t, err, nil)

	// not found
	pod = v1.Pod{
		TypeMeta: apis.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: apis.ObjectMeta{
			Name:         "pod00001",
			Namespace:    "default",
			UID:          "UID-POD-00001",
		},
		Spec:   v1.PodSpec{},
		Status: v1.PodStatus{},
	}

	appId, err = GenerateApplicationIdFromPod(&pod)
	assert.Equal(t, appId, "")
	assert.Assert(t, err != nil)
}

func newMockSchedulerApi() *MockSchedulerApi {
	return &MockSchedulerApi{
		registerFn: func(request *si.RegisterResourceManagerRequest, callback api.ResourceManagerCallback) (response *si.RegisterResourceManagerResponse, e error) {
			return nil, nil
		},
		updateFn: func(request *si.UpdateRequest) error {
			return nil
		},
	}
}
type MockSchedulerApi struct {
	callback api.ResourceManagerCallback
	registerFn func(request *si.RegisterResourceManagerRequest,
		callback api.ResourceManagerCallback) (*si.RegisterResourceManagerResponse, error)
	updateFn func(request *si.UpdateRequest) error
}

func (ms *MockSchedulerApi) RegisterResourceManager(request *si.RegisterResourceManagerRequest,
	callback api.ResourceManagerCallback) (*si.RegisterResourceManagerResponse, error) {
	return ms.registerFn(request, callback)
}

func (ms *MockSchedulerApi) Update(request *si.UpdateRequest) error {
	return ms.updateFn(request)
}

func assertAppState(t *testing.T, app *Application, expectedState string, duration time.Duration) {
	deadline := time.Now().Add(duration)
	for {
		if app.sm.Current() == expectedState {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for app %s reach to state %s", app.applicationId, expectedState)
		}
	}
}