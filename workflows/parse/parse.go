package parse

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/tektoncd/experimental/workflows/pkg/apis/workflows/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/client-go/kubernetes/scheme"
)

// MustParseWorkflow takes YAML and parses it into a *v1alpha1.Workflow
func ParseWorkflowOrDie(yaml []byte) *v1alpha1.Workflow {
	var w v1alpha1.Workflow
	meta := `apiVersion: tekton.dev/v1alpha1
kind: Workflow
`
	bytes := append([]byte(meta), yaml...)
	if _, _, err := scheme.Codecs.UniversalDeserializer().Decode(bytes, nil, &w); err != nil {
		panic(fmt.Sprintf("failed to parse workflow: %s", err))
	}
	return &w
}

// MustParseWorkflow takes YAML and parses it into a *v1alpha1.Workflow
func MustParseWorkflow(t *testing.T, yaml string) *v1alpha1.Workflow {
	var w v1alpha1.Workflow
	yaml = `apiVersion: tekton.dev/v1alpha1
kind: Workflow
` + yaml
	mustParseYAML(t, yaml, &w)
	return &w
}

func mustParseYAML(t *testing.T, yaml string, i runtime.Object) {
	if _, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(yaml), nil, i); err != nil {
		t.Fatalf("mustParseYAML (%s): %v", yaml, err)
	}
}

func MustParseWorkflowFromFile(t *testing.T, filename string) *v1alpha1.Workflow {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("error reading file: %+v", err)
	}

	sch := runtime.NewScheme()
	err = scheme.AddToScheme(sch)
	if err != nil {
		t.Fatalf("error creating scheme: %s", err)
	}

	decoder := streaming.NewDecoder(ioutil.NopCloser(bytes.NewReader(file)), serializer.NewCodecFactory(sch).UniversalDecoder())
	w := new(v1alpha1.Workflow)
	_, _, err = decoder.Decode(nil, w)
	if err != nil {
		if err != io.EOF {
			t.Fatalf("failed to parse workflow: %s", err)
		}
	}
	return w
}
