package v1alpha1_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/experimental/workflows/parse"
	"github.com/tektoncd/experimental/workflows/pkg/apis/workflows/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineparse "github.com/tektoncd/pipeline/test/parse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/ptr"
)

var ignoreTypeMeta = cmpopts.IgnoreFields(metav1.TypeMeta{}, "Kind", "APIVersion")
var ignoreObjectMeta = cmpopts.IgnoreFields(metav1.ObjectMeta{}, "Name", "Namespace", "GenerateName")

func sortWorkspaces(i, j *v1beta1.WorkspaceDeclaration) bool {
	return i.Name < j.Name
}

func sortPipelineWorkspaces(i, j *v1beta1.PipelineWorkspaceDeclaration) bool {
	return i.Name < j.Name
}

func sortParams(i, j *v1beta1.Param) bool {
	return i.Name < j.Name
}

func TestWorkflow_ToPipelineRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   v1alpha1.Workflow
		want *pipelinev1beta1.PipelineRun
	}{{
		name: "convert basic workflow spec to PR",
		in: v1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-workflow",
				Namespace: "my-namespace",
			},
			Spec: v1alpha1.WorkflowSpec{
				Params: []pipelinev1beta1.ParamSpec{{
					Name: "clone_sha",
					Default: &pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: "2aafa87e7cd14aef64956eba19721ce2fe814536",
					},
				}},
				Pipeline: v1alpha1.PipelineRef{
					Spec: pipelinev1beta1.PipelineSpec{
						Tasks: []pipelinev1beta1.PipelineTask{{
							Name: "clone-repo",
							TaskRef: &pipelinev1beta1.TaskRef{
								Name: "git-clone",
								Kind: "Task",
							},
						}},
						Params: []pipelinev1beta1.ParamSpec{{
							Name:        "clone_sha",
							Type:        pipelinev1beta1.ParamTypeString,
							Description: "Commit SHA to clone",
							Default:     nil,
						}},
						Workspaces: nil,
					},
				},
				ServiceAccountName: ptr.String("my-sa"),
			},
		},
		want: &pipelinev1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "my-workflow-run-",
				Namespace:    "my-namespace",
			},
			Spec: pipelinev1beta1.PipelineRunSpec{
				PipelineSpec: &pipelinev1beta1.PipelineSpec{
					Tasks: []pipelinev1beta1.PipelineTask{{
						Name: "clone-repo",
						TaskRef: &pipelinev1beta1.TaskRef{
							Name: "git-clone",
							Kind: "Task",
						},
					}},
					Params: []pipelinev1beta1.ParamSpec{{
						Name:        "clone_sha",
						Type:        pipelinev1beta1.ParamTypeString,
						Description: "Commit SHA to clone",
						Default:     nil,
					}},
					Workspaces: nil,
				},
				Params: []pipelinev1beta1.Param{{
					Name: "clone_sha",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: "2aafa87e7cd14aef64956eba19721ce2fe814536",
					},
				}},
				ServiceAccountName: "my-sa",
				Timeouts:           &pipelinev1beta1.TimeoutFields{},
			},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.ToPipelineRun()
			if err != nil {
				t.Fatalf("ToPipelineRun() err: %s", err)
			}
			if diff := cmp.Diff(tc.want, got, ignoreTypeMeta, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("ToPipelineRun() -want/+got: %s", diff)
			}
		})
	}
}

func TestWorkflowYamlToPipelineRun(t *testing.T) {
	wf := parse.MustParseWorkflow(t, `
apiVersion: tekton.dev/v1alpha1
kind: Workflow
metadata:
  name: ci-workflow
spec:
  pipeline:
    spec:
      workspaces:
      - name: source
      - name: github-app-private-key
      params:
      - name: repo-full-name
      - name: revision
      tasks:
      - name: create-check-runs
        taskRef:
          name: update-github-check-run
        params:
        - name: repo-full-name
          value: $(params.repo-full-name)
          # TODO: shouldn't need to add type
        workspaces:
        - name: github-app-private-key
          workspace: github-app-private-key
      - name: clone
        taskRef:
          name: git-clone
          bundle: gcr.io/tekton-releases/catalog/upstream/git-clone:0.7
        workspaces:
        - name: output
          workspace: source
        params:
        - name: url
          value: https://github.com/$(params.repo-full-name)
          type: string
        - name: revision
          value: $(params.revision)
          type: string
      - name: unit-tests
        taskRef:
          name: unit-tests
        workspaces:
        - name: source
          workspace: source
        runAfter:
        - clone
      finally:
      - name: post-github-status
        taskRef:
          name: update-github-check-run
        params:
        - name: status
          value: completed
        workspaces:
        - name: github-app-private-key
          workspace: github-app-private-key
  params:
  - name: repo-full-name
    default: lbernick/pipeline
  serviceAccountName: container-registry-sa
  workspaces:
  #- name: github-app-private-key
    # does this end up being a workflows secret or a pipelines secret?
    #secret:
      #secretName: github-app-key
  - name: source
    volumeClaimTemplate:
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 1Gi    
`)

	want := pipelineparse.MustParsePipelineRun(t, `
spec:
  pipelineSpec:
    workspaces:
    - name: source
    - name: github-app-private-key
    params:
    - name: repo-full-name
    - name: revision
    tasks:
    - name: create-check-runs
      taskRef:
        name: update-github-check-run
      params:
      - name: repo-full-name
        value: $(params.repo-full-name)
      workspaces:
      - name: github-app-private-key
        workspace: github-app-private-key
    - name: clone
      taskRef:
        name: git-clone
        bundle: gcr.io/tekton-releases/catalog/upstream/git-clone:0.7
      workspaces:
      - name: output
        workspace: source
      params:
      - name: url
        value: https://github.com/$(params.repo-full-name)
        type: string
      - name: revision
        value: $(params.revision)
        type: string
    - name: unit-tests
      taskRef:
        name: unit-tests
      workspaces:
      - name: source
        workspace: source
      runAfter:
      - clone
    finally:
    - name: post-github-status
      taskRef:
        name: update-github-check-run
      params:
      - name: status
        value: completed
      workspaces:
      - name: github-app-private-key
        workspace: github-app-private-key
  serviceAccountName: container-registry-sa
  timeouts: {}
  params:
  - name: repo-full-name
    value: lbernick/pipeline
  workspaces:
  #- name: github-app-private-key
    # does this end up being a workflows secret or a pipelines secret?
    #secret:
      #secretName: github-app-key
  - name: source
    volumeClaimTemplate:
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 1Gi  
`)
	got, err := wf.ToPipelineRun()
	if err != nil {
		t.Fatalf("ToPipelineRun() err: %s", err)
	}
	opts := []cmp.Option{ignoreTypeMeta, ignoreObjectMeta, cmpopts.EquateEmpty(), cmpopts.SortSlices(sortWorkspaces), cmpopts.SortSlices(sortPipelineWorkspaces), cmpopts.SortSlices(sortParams)}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("ToPipelineRun() -want/+got: %s", diff)
	}
}
