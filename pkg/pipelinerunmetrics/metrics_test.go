/*
Copyright 2019 The Tekton Authors

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

package pipelinerunmetrics

import (
	"sync"
	"testing"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	fakepipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipelinerun/fake"
	"github.com/tektoncd/pipeline/pkg/names"
	ttesting "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/metrics/metricstest" // Required to setup metrics env for testing
	_ "knative.dev/pkg/metrics/testing"
)

var (
	startTime      = metav1.Now()
	completionTime = metav1.NewTime(startTime.Time.Add(time.Minute))
)

func TestUninitializedMetrics(t *testing.T) {
	metrics := Recorder{}

	if err := metrics.DurationAndCount(&v1beta1.PipelineRun{}); err == nil {
		t.Error("DurationAndCount recording expected to return error but got nil")
	}
	if err := metrics.RunningPipelineRuns(nil); err == nil {
		t.Error("Current PR count recording expected to return error but got nil")
	}
}

func TestRecordPipelineRunDurationCount(t *testing.T) {
	for _, test := range []struct {
		name              string
		pipelineRun       *v1beta1.PipelineRun
		expectedTags      map[string]string
		expectedCountTags map[string]string
		expectedDuration  float64
		expectedCount     int64
	}{{
		name: "for succeeded pipeline",
		pipelineRun: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
			Spec: v1beta1.PipelineRunSpec{
				PipelineRef: &v1beta1.PipelineRef{Name: "pipeline-1"},
			},
			Status: v1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionTrue,
					}},
				},
				PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"namespace":   "ns",
			"status":      "success",
		},
		expectedCountTags: map[string]string{
			"status": "success",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}, {
		name: "for cancelled pipeline",
		pipelineRun: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
			Spec: v1beta1.PipelineRunSpec{
				PipelineRef: &v1beta1.PipelineRef{Name: "pipeline-1"},
			},
			Status: v1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionFalse,
						Reason: ReasonCancelled,
					}},
				},
				PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"namespace":   "ns",
			"status":      "cancelled",
		},
		expectedCountTags: map[string]string{
			"status": "cancelled",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}, {
		name: "for failed pipeline",
		pipelineRun: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "pipelinerun-1", Namespace: "ns"},
			Spec: v1beta1.PipelineRunSpec{
				PipelineRef: &v1beta1.PipelineRef{Name: "pipeline-1"},
			},
			Status: v1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionFalse,
					}},
				},
				PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"namespace":   "ns",
			"status":      "failed",
		},
		expectedCountTags: map[string]string{
			"status": "failed",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}} {
		t.Run(test.name, func(t *testing.T) {
			unregisterMetrics()

			metrics, err := NewRecorder()
			if err != nil {
				t.Fatalf("NewRecorder: %v", err)
			}

			if err := metrics.DurationAndCount(test.pipelineRun); err != nil {
				t.Errorf("DurationAndCount: %v", err)
			}
			metricstest.CheckDistributionData(t, "pipelinerun_duration_seconds", test.expectedTags, 1, test.expectedDuration, test.expectedDuration)
			metricstest.CheckCountData(t, "pipelinerun_count", test.expectedCountTags, test.expectedCount)

		})
	}
}

func TestRecordRunningPipelineRunsCount(t *testing.T) {
	unregisterMetrics()

	newPipelineRun := func(status corev1.ConditionStatus) *v1beta1.PipelineRun {
		return &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: names.SimpleNameGenerator.RestrictLengthWithRandomSuffix("pipelinerun-")},
			Status: v1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: status,
					}},
				},
			},
		}
	}

	ctx, _ := ttesting.SetupFakeContext(t)
	informer := fakepipelineruninformer.Get(ctx)
	// Add N randomly-named PipelineRuns with differently-succeeded statuses.
	for _, tr := range []*v1beta1.PipelineRun{
		newPipelineRun(corev1.ConditionTrue),
		newPipelineRun(corev1.ConditionUnknown),
		newPipelineRun(corev1.ConditionFalse),
	} {
		if err := informer.Informer().GetIndexer().Add(tr); err != nil {
			t.Fatalf("Adding TaskRun to informer: %v", err)
		}
	}

	metrics, err := NewRecorder()
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := metrics.RunningPipelineRuns(informer.Lister()); err != nil {
		t.Errorf("RunningPipelineRuns: %v", err)
	}
	metricstest.CheckLastValueData(t, "running_pipelineruns_count", map[string]string{}, 1)

}

func unregisterMetrics() {
	metricstest.Unregister("pipelinerun_duration_seconds", "pipelinerun_count", "running_pipelineruns_count")

	// Allow the recorder singleton to be recreated.
	once = sync.Once{}
	r = nil
	recorderErr = nil
}
