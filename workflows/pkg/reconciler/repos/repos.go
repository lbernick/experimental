package repos

import (
	"context"

	"github.com/tektoncd/experimental/workflows/pkg/apis/workflows/v1alpha1"
	reposreconciler "github.com/tektoncd/experimental/workflows/pkg/client/injection/reconciler/workflows/v1alpha1/gitrepository"
	"knative.dev/pkg/reconciler"
)

type Reconciler struct {
}

var _ reposreconciler.Interface = (*Reconciler)(nil)

func (r *Reconciler) ReconcileKind(ctx context.Context, w *v1alpha1.GitRepository) reconciler.Event {
	// TODO: somehow create a new connector like the pipelines resolver
	// have it create a Knative eventsource w/ owner ref
	// knative eventsource status should be reflected back to the repo status
	return nil
}
