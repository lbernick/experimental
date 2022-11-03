package connectors

import (
	"context"

	"github.com/tektoncd/experimental/workflows/pkg/apis/workflows/v1alpha1"
)

type Connector interface {
	Name() string

	GetSelector(context.Context) map[string]string

	GetSchema(context.Context) interface{} // Returns an empty object the knative eventsource can be deserialized into

	ToKnativeEventSource(context.Context, v1alpha1.GitRepository) interface{} // There doesn't appear to be an interface for this
}
