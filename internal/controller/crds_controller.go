/*
Copyright 2025.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
	http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	webappv1 "github.com/rachit4645/KubeCRDs/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const appFinalizer = "apps.example.com/finalizer"

// +kubebuilder:rbac:groups=apps.example.com,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.example.com,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.example.com,resources=apps/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update;get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the App object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("AppReconciler").WithValues("app", req.NamespacedName)
	logger.Info("Starting reconciliation")

	var app webappv1.App
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("App resource not found, might be deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	namespace := app.Namespace
	if namespace == "" {
		// fallback to default namespace if empty
		app.Namespace = "kubecrds-system"
	}

	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(app.Finalizers, appFinalizer) {
			logger.Info("Adding finalizer", "finalizer", appFinalizer)
			app.Finalizers = append(app.Finalizers, appFinalizer)
			if err := r.Update(ctx, &app); err != nil {
				logger.Error(err, "Failed to add finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		// App is being deleted
		if containsString(app.Finalizers, appFinalizer) {
			logger.Info("Deleting Deployment for App", "deployment", app.Name)
			if err := r.deleteDeployment(ctx, &app); err != nil {
				return ctrl.Result{}, err
			}
			app.Finalizers = removeString(app.Finalizers, appFinalizer)
			if err := r.Update(ctx, &app); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 3. Reconcile Deployment
	deploy, err := r.buildDeployment(&app)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileDeployment(ctx, deploy); err != nil {
		return ctrl.Result{}, err
	}
	// 4. Update status
	if err := r.updateAppStatus(ctx, &app); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AppReconciler) buildDeployment(app *webappv1.App) (*appsv1.Deployment, error) {
	logger := ctrl.Log.WithName("AppReconciler").WithValues("app", app.Namespace+"/"+app.Name)
	logger.Info("Building Deployment spec", "image", app.Spec.Image, "replicas", *app.Spec.Replicas, "namespace", app.Namespace)
	replicas := int32(*app.Spec.Replicas)
	if app.Spec.Replicas != nil {
		replicas = *app.Spec.Replicas
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": app.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": app.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: app.Spec.Image,
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(app, deploy, r.Scheme); err != nil {
		return nil, err
	}
	return deploy, nil
}

func (r *AppReconciler) updateAppStatus(ctx context.Context, app *webappv1.App) error {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, &deploy); err != nil {
		if errors.IsNotFound(err) {
			app.Status.AvailableReplicas = 0
		} else {
			return err
		}
	} else {
		app.Status.AvailableReplicas = deploy.Status.AvailableReplicas
	}
	return r.Status().Update(ctx, app)
}

func (r *AppReconciler) reconcileDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}
	// Patch only spec.replicas and container image
	existing.Spec.Replicas = desired.Spec.Replicas
	if len(existing.Spec.Template.Spec.Containers) > 0 && len(desired.Spec.Template.Spec.Containers) > 0 {
		existing.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
	}
	return r.Update(ctx, &existing)
}

func (r *AppReconciler) deleteDeployment(ctx context.Context, app *webappv1.App) error {
	var deploy appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, &deploy)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	return r.Delete(ctx, &deploy)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.App{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Named("crds").
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

//Group Version
//Finalizer
//DeployIt
//Reconsile Loop - How it work
