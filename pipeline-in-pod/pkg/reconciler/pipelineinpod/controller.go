package pipelineinpod

import (
	context "context"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	run "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/run"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipelinerun"
	v1alpha1run "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	pipelinecontroller "github.com/tektoncd/pipeline/pkg/controller"
	tkncontroller "github.com/tektoncd/pipeline/pkg/controller"
	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	configmap "knative.dev/pkg/configmap"
	controller "knative.dev/pkg/controller"
	logging "knative.dev/pkg/logging"
)

const (
	ControllerName = "pipelineinpod-controller"
	kind           = "PipelineInPod"
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	pipelineClientSet := pipelineclient.Get(ctx)
	kubeClientSet := kubeclient.Get(ctx)
	runInformer := run.Get(ctx)
	pipelineRunInformer := pipelineruninformer.Get(ctx)

	r := &Reconciler{
		pipelineClientSet: pipelineClientSet,
		kubeClientSet:     kubeClientSet,
		runLister:         runInformer.Lister(),
		pipelineRunLister: pipelineRunInformer.Lister(),
	}

	impl := v1alpha1run.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options {
		return controller.Options{
			AgentName: ControllerName,
		}
	})

	logger.Info("Setting up event handlers")

	// Add event handler for Runs
	runInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: tkncontroller.FilterRunRef(v1beta1.SchemeGroupVersion.String(), kind),
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	// Add event handler for PipelineRuns controlled by Run
	// TODO: why filter to kind Pipeline? is it because you're filtering by Owner
	pipelineRunInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: pipelinecontroller.FilterOwnerRunRef(runInformer.Lister(), v1beta1.SchemeGroupVersion.String(), kind),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	})

	return impl
}
