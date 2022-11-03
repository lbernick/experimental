package repos

import (
	"context"

	reposinformer "github.com/tektoncd/experimental/workflows/pkg/client/injection/informers/workflows/v1alpha1/gitrepository"
	reposreconciler "github.com/tektoncd/experimental/workflows/pkg/client/injection/reconciler/workflows/v1alpha1/gitrepository"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	reposInformer := reposinformer.Get(ctx)
	r := &Reconciler{}
	impl := reposreconciler.NewImpl(ctx, r)
	reposInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	// TODO: There's not really an EventSource informer because it's not a Kind...
	// Probably each connector will need its own Controller
	return impl
}
