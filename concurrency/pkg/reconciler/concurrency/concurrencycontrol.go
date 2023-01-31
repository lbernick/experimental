package concurrency

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/tektoncd/experimental/concurrency/pkg/apis/concurrency/v1alpha1"
	"github.com/tektoncd/experimental/concurrency/pkg/apis/config"
	listersv1alpha1 "github.com/tektoncd/experimental/concurrency/pkg/client/listers/concurrency/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"golang.org/x/sync/errgroup"
	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/controller"
	logging "knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
)

// Reconciler implements controller.Reconciler
type Reconciler struct {
	ConcurrencyControlLister listersv1alpha1.ConcurrencyControlLister
	PipelineClientSet        clientset.Interface
	PipelineRunLister        listers.PipelineRunLister
}

var (
	cancelPipelineRunPatchBytes           []byte
	gracefullyCancelPipelineRunPatchBytes []byte
	gracefullyStopPipelineRunPatchBytes   []byte
	concurrencyControlsAppliedLabel       = "tekton.dev/concurrency"
)

func init() {
	var err error
	cancelPipelineRunPatchBytes, err = json.Marshal([]jsonpatch.JsonPatchOperation{
		{
			Operation: "add",
			Path:      "/spec/status",
			Value:     v1beta1.PipelineRunSpecStatusCancelled,
		}})
	if err != nil {
		log.Fatalf("failed to marshal PipelineRun cancel patch bytes: %v", err)
	}
	gracefullyCancelPipelineRunPatchBytes, err = json.Marshal([]jsonpatch.JsonPatchOperation{
		{
			Operation: "add",
			Path:      "/spec/status",
			Value:     v1beta1.PipelineRunSpecStatusCancelledRunFinally,
		}})
	if err != nil {
		log.Fatalf("failed to marshal PipelineRun gracefully cancel patch bytes: %v", err)
	}
	gracefullyStopPipelineRunPatchBytes, err = json.Marshal([]jsonpatch.JsonPatchOperation{
		{
			Operation: "add",
			Path:      "/spec/status",
			Value:     v1beta1.PipelineRunSpecStatusStoppedRunFinally,
		}})
	if err != nil {
		log.Fatalf("failed to marshal PipelineRun gracefully stop patch bytes: %v", err)
	}
}

// ReconcileKind reconciles PipelineRuns
func (r *Reconciler) ReconcileKind(ctx context.Context, pr *v1beta1.PipelineRun) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	cfg := config.FromContext(ctx)
	if len(cfg.AllowedNamespaces) > 0 && !cfg.AllowedNamespaces.Has(pr.Namespace) {
		logger.Infof("PipelineRun %s/%s is not in an allowed namespace, skipping concurrency controls", pr.Namespace, pr.Name)
		return nil
	}
	if !pr.IsPending() || concurrencyControlsPreviouslyApplied(pr) {
		return nil
	}

	// Find all concurrency controls in the namespace and determine which ones match
	ccs, err := r.ConcurrencyControlLister.ConcurrencyControls(pr.Namespace).List(k8slabels.Everything())
	if err != nil {
		return err
	}
	logger.Debugf("found %d concurrency controls in namespace %s", len(ccs), pr.Namespace)

	var matchingCCs []*v1alpha1.ConcurrencyControl
	var mismatchingCCs []*v1alpha1.ConcurrencyControl
	var missingLabelsCCs []*v1alpha1.ConcurrencyControl

	// If a concurrency control matches the PipelineRun, it should cancel other PipelineRuns in the same group
	// during this reconcile loop.
	// If the PipelineRun has different values for the labels specified in the concurrency control's label selector,
	// the concurrency control isn't relevant to this PipelineRun.
	// However, some labels may be added by a reconciler that runs concurrently.
	// For example, the PipelineRun controller adds the label "tekton.dev/pipeline".
	// If the concurrency control matches on this label and the PipelineRun doesn't have it,
	// It's possible that this label will be added later.
	// If no concurrency controls match, but some of them don't match *yet*,
	// The PipelineRun won't be marked as having completed concurrency controls, and this controller will check again
	// when the PipelineRun is re-reconciled.
	// If some concurrency controls match, this doesn't work
	// This is a bit of a hack because it may just result in reconciling the same PipelineRun many times but alas
	for _, cc := range ccs {
		if Matches(pr, cc) {
			matchingCCs = append(matchingCCs, cc)
		} else if allLabelsPresent(pr, cc) {
			mismatchingCCs = append(mismatchingCCs, cc)
		} else {
			missingLabelsCCs = append(missingLabelsCCs, cc)
		}
	}

	prsToCancel := sets.NewString()
	var strategy v1alpha1.Strategy
	for _, cc := range ccs {
		// If a concurrency control matches the PipelineRun, cancel concurrent PipelineRuns in the same group.
		// If it does not match the PipelineRun and all labels are present,

		if !Matches(pr, cc) {
			// Concurrency control does not apply to this PipelineRun
			continue
		}
		logger.Infof("found concurrency control %s matching PipelineRun %s/%s", cc.Name, pr.Namespace, pr.Name)

		// If concurrency control matches the current pipelinerun, get all pipelineruns matching the same label selector
		// and with the same values for label keys in groupby. Cancel them all except the one currently running.
		labelSelector, err := getLabelSelector(cc, pr)
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("error building label selector from concurrency control: %s", err))
		}
		matchingPRs, err := r.PipelineRunLister.PipelineRuns(pr.Namespace).List(labelSelector)
		if err != nil {
			return err
		}
		if strategy == "" {
			strategy = v1alpha1.Strategy(cc.Spec.Strategy)
		} else if string(strategy) != cc.Spec.Strategy {
			// This error is unlikely to be fixed by retrying
			return controller.NewPermanentError(fmt.Errorf("found multiple concurrency strategies for PipelineRun %s in namespace %s; skipping concurrency controls", pr.Name, pr.Namespace))
		}
		for _, matchingPR := range matchingPRs {
			if matchingPR.Name == pr.Name {
				continue
			}
			if matchingPR.IsDone() {
				logger.Debugf("skipping cancelation of completed PR %s/%s", pr.Namespace, pr.Name)
				continue
			}
			prsToCancel.Insert(matchingPR.Name)
		}
	}
	err = r.cancelPipelineRuns(ctx, pr.Namespace, prsToCancel.List(), strategy)
	if err != nil {
		return fmt.Errorf("error canceling PipelineRuns in the same concurrency group as %s: %s", pr.Name, err)
	}

	return r.updateLabelsAndStartPipelineRun(ctx, pr)
}

// allLabelsPresent returns True if the PipelineRun has all the labels
// in cc.spec.selector.matchLabels and cc.spec.groupBy; false otherwise.
func allLabelsPresent(pr *v1beta1.PipelineRun, cc *v1alpha1.ConcurrencyControl) bool {
	presentLabels := k8slabels.Set(pr.Labels)
	for k := range cc.Spec.Selector.MatchLabels {
		if !presentLabels.Has(k) {
			return false
		}
	}
	return true
}

/*
// getLabelSelector returns a label selector for PipelineRuns matching the ConcurrencyControl's selector
// and that have values for the labels specified by the ConcurrencyControl's groupBy.
func GetLabelSelector(cc *v1alpha1.ConcurrencyControl) (k8slabels.Selector, error) {
	labelSelector := cc.Spec.Selector.MatchLabels
	var requirements []k8slabels.Requirement
	for _, key := range cc.Spec.GroupBy {
		r, err := k8slabels.NewRequirement(key, selection.Exists, []string{})
		if err != nil || r == nil {
			return nil, fmt.Errorf("error building label selector")
		}
		requirements = append(requirements, *r)

	}
	out := k8slabels.SelectorFromSet(labelSelector)
	out = out.Add(requirements...)
	return out, nil
} */

// Matches returns true if the PipelineRun is selected by the ConcurrencyControl's selector.
// An empty selector always matches the PipelineRun.
func Matches(pr *v1beta1.PipelineRun, cc *v1alpha1.ConcurrencyControl) bool {
	// TODO: Support MatchExpressions as well
	return k8slabels.SelectorFromSet(cc.Spec.Selector.MatchLabels).Matches(k8slabels.Set(pr.Labels))
}

/*
// getLabelSelector returns a label selector for PipelineRuns matching the ConcurrencyControl's selector
// and that have the same value for the input PipelineRun's labels specified by the ConcurrencyControl's groupBy.
// If the input PipelineRun does not have a value for the key specified by groupBy, the label selector returned
// will select PipelineRuns that also do not have a value for the key specified by groupBy.
func getLabelSelector(cc *v1alpha1.ConcurrencyControl, pr *v1beta1.PipelineRun) (k8slabels.Selector, error) {
	labelSelector := cc.Spec.Selector.MatchLabels
	var requirements []k8slabels.Requirement
	for _, key := range cc.Spec.GroupBy {
		val, ok := pr.Labels[key]
		if !ok {
			r, err := k8slabels.NewRequirement(key, selection.DoesNotExist, []string{})
			if err != nil {
				return nil, err
			}
			if r == nil {
				return nil, fmt.Errorf("error building label selector")
			}
			requirements = append(requirements, *r)
		} else {
			labelSelector[key] = val
		}
	}
	out := k8slabels.SelectorFromSet(labelSelector)
	out = out.Add(requirements...)
	return out, nil
} */

// getLabelSelector returns a label selector for PipelineRuns matching the ConcurrencyControl's selector
// and that have the same value for the input PipelineRun's labels specified by the ConcurrencyControl's groupBy.
// It returns an error if the input PipelineRun does not have a value for the key specified by groupBy.
func getLabelSelector(cc *v1alpha1.ConcurrencyControl, pr *v1beta1.PipelineRun) (k8slabels.Selector, error) {
	labelSelector := cc.Spec.Selector.MatchLabels
	var requirements []k8slabels.Requirement
	for _, key := range cc.Spec.GroupBy {
		val, ok := pr.Labels[key]
		if !ok {
			return nil, fmt.Errorf("PipelineRun %s/%s missing label %s", pr.Namespace, pr.Name, val)
		}
		labelSelector[key] = val
	}
	out := k8slabels.SelectorFromSet(labelSelector)
	out = out.Add(requirements...)
	return out, nil
}

// concurrencyControlsPreviouslyApplied returns true if concurrency controls have been applied in a previous reconcile loop,
// and no further work is necessary
func concurrencyControlsPreviouslyApplied(pr *v1beta1.PipelineRun) bool {
	_, ok := pr.Labels[concurrencyControlsAppliedLabel]
	return ok
}

func (r *Reconciler) cancelPipelineRuns(ctx context.Context, namespace string, names []string, strategy v1alpha1.Strategy) error {
	logger := logging.FromContext(ctx)
	g := new(errgroup.Group)
	for _, n := range names {
		n := n // https://go.dev/doc/faq#closures_and_goroutines
		g.Go(func() error {
			logger.Infof("canceling PipelineRun %s in namespace %s", n, namespace)
			return r.cancelPipelineRun(ctx, namespace, n, strategy)
		})
	}
	// TODO: We may want to implement a solution that avoids blocking until all PipelineRuns have been canceled.
	// However, this is probably good enough for the time being.
	// This is similar to how the PipelineRun reconciler cancels child TaskRuns
	// (see https://github.com/tektoncd/pipeline/blob/main/pkg/reconciler/pipelinerun/cancel.go).
	return g.Wait()
}

func (r *Reconciler) cancelPipelineRun(ctx context.Context, namespace, name string, s v1alpha1.Strategy) error {
	var bytes []byte
	switch s {
	case v1alpha1.StrategyCancel:
		bytes = cancelPipelineRunPatchBytes
	case v1alpha1.StrategyGracefullyCancel:
		bytes = gracefullyCancelPipelineRunPatchBytes
	case v1alpha1.StrategyGracefullyStop:
		bytes = gracefullyStopPipelineRunPatchBytes
	default:
		return fmt.Errorf("unsupported operation: %s", s)
	}
	_, err := r.PipelineClientSet.TektonV1beta1().PipelineRuns(namespace).Patch(ctx, name, types.JSONPatchType, bytes, metav1.PatchOptions{})
	if errors.IsNotFound(err) {
		// The PipelineRun may have been deleted in the meantime
		return nil
	} else if err != nil {
		return fmt.Errorf("error patching PipelineRun %s using strategy %s: %s", name, s, err)
	}
	return nil
}

// updateLabelsAndStartPipelineRun marks the PipelineRun with a label indicating concurrency controls have been applied.
// If it was modified to be pending by the mutating admission webhook (rather than started as pending by the user),
// it starts the PipelineRun.
func (r *Reconciler) updateLabelsAndStartPipelineRun(ctx context.Context, pr *v1beta1.PipelineRun) error {
	newPR, err := r.PipelineRunLister.PipelineRuns(pr.Namespace).Get(pr.Name)
	if err != nil {
		return fmt.Errorf("error getting PipelineRun %s in namespace %s when updating labels: %w", pr.Name, pr.Namespace, err)
	}
	newPR = newPR.DeepCopy()
	newPR.Labels = pr.Labels
	if len(newPR.ObjectMeta.Labels) == 0 {
		newPR.ObjectMeta.Labels = make(map[string]string)
	}
	newPR.Labels[concurrencyControlsAppliedLabel] = "true"
	if _, ok := newPR.Labels[v1alpha1.LabelToStartPR]; ok {
		delete(newPR.Labels, v1alpha1.LabelToStartPR)
		// This PipelineRun was marked as pending by the mutating admission webhook, not the user. OK to start it.
		newPR.Spec.Status = ""
	}
	_, err = r.PipelineClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Update(ctx, newPR, metav1.UpdateOptions{})
	return err
}
