package parse_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/experimental/workflows/parse"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestParseWorkflowFromFile(t *testing.T) {
	filename := "../examples/dogfood/workflow.yaml"
	got := parse.MustParseWorkflowFromFile(t, filename)
	opts := []cmp.Option{ignoreTypeMeta, ignoreObjectMeta, cmpopts.EquateEmpty(), cmpopts.SortSlices(sortWorkspaces), cmpopts.SortSlices(sortPipelineWorkspaces), cmpopts.SortSlices(sortParams)}
	want := parse.MustParseWorkflow(t, `
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
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("ToPipelineRun() -want/+got: %s", diff)
	}
}
