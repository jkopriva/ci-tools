package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientset "k8s.io/client-go/kubernetes/typed/core/v1"

	imageclientset "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/ci-operator/pkg/api"
)

type testStep struct {
	config    api.TestStepConfiguration
	podClient coreclientset.PodsGetter
	istClient imageclientset.ImageStreamTagsGetter
	jobSpec   *JobSpec
}

func (s *testStep) Inputs(ctx context.Context, dry bool) (api.InputDefinition, error) {
	return nil, nil
}

func (s *testStep) Run(ctx context.Context, dry bool) error {
	log.Printf("Executing test %s", s.config.As)

	pod := &coreapi.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: s.config.As,
		},
		Spec: coreapi.PodSpec{
			RestartPolicy: coreapi.RestartPolicyNever,
			Containers: []coreapi.Container{
				{
					Name:    "test",
					Image:   fmt.Sprintf("%s:%s", PipelineImageStream, s.config.From),
					Command: []string{"/bin/bash", "-c", "#!/bin/bash\nset -euo pipefail\n" + s.config.Commands},
				},
			},
		},
	}
	if owner := s.jobSpec.Owner(); owner != nil {
		pod.OwnerReferences = append(pod.OwnerReferences, *owner)
	}

	if dry {
		j, _ := json.MarshalIndent(pod, "", "  ")
		log.Printf("pod:\n%s", j)
		return nil
	}

	go func() {
		<-ctx.Done()
		log.Printf("cleanup: Deleting test pod %s", s.config.As)
		if err := s.podClient.Pods(s.jobSpec.Namespace()).Delete(s.config.As, nil); err != nil && !errors.IsNotFound(err) {
			log.Printf("error: Could not delete test pod: %v", err)
		}
	}()

	pod, err := createOrRestartPod(s.podClient.Pods(s.jobSpec.Namespace()), pod)
	if err != nil {
		return err
	}

	if err := waitForPodCompletion(s.podClient.Pods(s.jobSpec.Namespace()), pod.Name); err != nil {
		return err
	}

	return nil
}

func (s *testStep) Done() (bool, error) {
	ready, err := isPodCompleted(s.podClient.Pods(s.jobSpec.Namespace()), s.config.As)
	if err != nil {
		return false, err
	}
	if !ready {
		return false, nil
	}
	return true, nil
}

func (s *testStep) Requires() []api.StepLink {
	return []api.StepLink{api.InternalImageLink(s.config.From)}
}

func (s *testStep) Creates() []api.StepLink {
	return []api.StepLink{}
}

func (s *testStep) Provides() (api.ParameterMap, api.StepLink) {
	return nil, nil
}

func (s *testStep) Name() string { return s.config.As }

func TestStep(config api.TestStepConfiguration, podClient coreclientset.PodsGetter, jobSpec *JobSpec) api.Step {
	return &testStep{
		config:    config,
		podClient: podClient,
		jobSpec:   jobSpec,
	}
}
