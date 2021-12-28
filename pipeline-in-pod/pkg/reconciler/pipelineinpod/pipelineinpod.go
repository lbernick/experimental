package pipelineinpod

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	listersalpha "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1alpha1"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/reconciler/events"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const (
	// ReasonRunFailedValidation indicates that the reason for failure status is that Run failed validation
	ReasonRunFailedValidation = "ReasonRunFailedValidation"

	// ReasonRunFailedCreatingPipelineRun indicates that the reason for failure status is that Run failed
	// to create PipelineRun
	ReasonRunFailedCreatingPipelineRun = "ReasonRunFailedCreatingPipelineRun"
)

// Reconciler implements controller.Reconciler for Run resources.
type Reconciler struct {
	pipelineClientSet clientset.Interface
	kubeClientSet     kubernetes.Interface
	runLister         listersalpha.RunLister
	pipelineRunLister listers.PipelineRunLister
}

// Check that our Reconciler implements Interface
var _ run.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, run *v1alpha1.Run) reconciler.Event {
	logger := logging.FromContext(ctx)

	if run.Spec.Ref == nil ||
		run.Spec.Ref.APIVersion != v1alpha1.SchemeGroupVersion.String() || run.Spec.Ref.Kind != kind {
		logger.Warn("Should not have been notified about Run %s/%s; will do nothing", run.Namespace, run.Name)
		return nil
	}

	logger.Infof("Reconciling Run %s/%s at %v", run.Namespace, run.Name, time.Now())

	// If the Run has not started, initialize the Condition and set the start time.
	if !run.HasStarted() {
		logger.Infof("Starting new Run %s/%s", run.Namespace, run.Name)
		run.Status.InitializeConditions()
		// In case node time was not synchronized, when controller has been scheduled to other nodes.
		if run.Status.StartTime.Sub(run.CreationTimestamp.Time) < 0 {
			logger.Warnf("Run %s/%s createTimestamp %s is after the Run started %s", run.Namespace, run.Name, run.CreationTimestamp, run.Status.StartTime)
			run.Status.StartTime = &run.CreationTimestamp
		}
		// Send the "Started" event
		afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
		events.Emit(ctx, nil, afterCondition, run)
	}

	if run.IsDone() {
		logger.Infof("Run %s/%s is done", run.Namespace, run.Name)
		return nil
	}

	var merr error

	beforeCondition := run.Status.GetCondition(apis.ConditionSucceeded)

	if err := r.reconcile(ctx, run); err != nil {
		logger.Errorf("Reconcile error: %v", err.Error())
		merr = multierror.Append(merr, controller.NewPermanentError(err))
	}

	if err := r.updateLabelsAndAnnotations(ctx, run); err != nil {
		logger.Warn("Failed to update Run labels/annotations", zap.Error(err))
		merr = multierror.Append(merr, err)
	}

	afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
	events.Emit(ctx, beforeCondition, afterCondition, run)

	// Only transient errors that should retry the reconcile are returned
	return merr
}

func (r *Reconciler) reconcile(ctx context.Context, run *v1alpha1.Run) error {
	logger := logging.FromContext(ctx)

	// confirm the run spec is valid
	if err := validate(run); err != nil {
		logger.Errorf("Run %s/%s is invalid because of %v", run.Namespace, run.Name, err)
		run.Status.MarkRunFailed(ReasonRunFailedValidation,
			"Run can't be run because it has an invalid spec - %v", err)
		return controller.NewPermanentError(fmt.Errorf("run %s/%s is invalid because of %v", run.Namespace, run.Name, err))
	}

	// TODO
	// If run is managing pipelineruns:
	// Get the pipelinerun if it exists
	// if it does exist already, get the pods it runs?
	// update pipelinerun status to

	//update the run's status to reflect it, return
	// Get and validate the pipeline
	// get and validate all the tasks in the pipeline

	// If run is managing pods:
	// Based on the run's name, get and validate the pipeline

	// If we're reconciling pipelineruns:
	// get the pipe
	return nil
}

func validate(run *v1alpha1.Run) (errs *apis.FieldError) {
	if run.Spec.Ref.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	}
	return errs
}

func (r *Reconciler) updateLabelsAndAnnotations(ctx context.Context, run *v1alpha1.Run) error {
	// TODO
	return nil
}
