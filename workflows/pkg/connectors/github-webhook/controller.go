package githubwebhook

import (
	"context"

	reposinformer "github.com/tektoncd/experimental/workflows/pkg/client/injection/informers/workflows/v1alpha1/gitrepository"
	reposreconciler "github.com/tektoncd/experimental/workflows/pkg/client/injection/reconciler/workflows/v1alpha1/gitrepository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
)

var filterFunc = func(obj interface{}) bool {
	if object, ok := obj.(metav1.Object); ok {
		labels := object.GetLabels()
		v, ok := labels["workflows.tekton.dev/connector"]
		if ok && v == "github-webhook" {
			return true
		}
	}
	return false
}

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	reposInformer := reposinformer.Get(ctx)
	r := &Reconciler{}
	impl := reposreconciler.NewImpl(ctx, r)

	reposInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: filterFunc,
		Handler:    controller.HandleAll(impl.Enqueue),
	})
	return impl
}
