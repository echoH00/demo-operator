/*
Copyright 2024.

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
	"fmt"
	echohv1alpha1 "github.com/echoH00/operator/demo/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// MyAppReconciler reconciles a MyApp object
type MyAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// MyApp Finalizer
const MyAppFinalizer = "echoh.wonderscloud.com/finalizer"

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=echoh.wonderscloud.com,resources=myapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=echoh.wonderscloud.com,resources=myapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=echoh.wonderscloud.com,resources=myapps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MyApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *MyAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	myapp := &echohv1alpha1.MyApp{}
	err := r.Get(ctx, req.NamespacedName, myapp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("MyApp Obj has been deleted...")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MyApp obj///")
		return ctrl.Result{}, err
	}

	// set status Unknow when no status available
	if myapp.Status.Conditions == nil || len(myapp.Status.Conditions) == 0 {
		meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
			Type:    fmt.Sprint("Available"),
			Status:  metav1.ConditionUnknown,
			Reason:  "conditionsNilLenConditions0",
			Message: "conditions nil lenconditions 0, reconciling",
		})
		// update status
		if err := r.Status().Update(ctx, myapp); err != nil {
			log.Error(err, "reconcile update status Failed")
			return ctrl.Result{}, err
		}
		// get the latest status
		if err := r.Get(ctx, req.NamespacedName, myapp); err != nil {
			log.Error(err, "Failed to re-fetch MyApp")
			return ctrl.Result{}, err
		}
	}

	// add finalizer
	if !controllerutil.ContainsFinalizer(myapp, MyAppFinalizer) {
		if ok := controllerutil.AddFinalizer(myapp, MyAppFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into MyApp custom resource")
			return ctrl.Result{}, err
		}
		// update MyApp obj
		if err := r.Update(ctx, myapp); err != nil {
			log.Error(err, "Failed to update MyApp custom resource with finalizer")
			return ctrl.Result{}, err
		}
	}

	// check if the MyApp instance is marked to be delete
	isMarkedToBeDeleted := myapp.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		// if has finalizer
		if controllerutil.ContainsFinalizer(myapp, MyAppFinalizer) {
			log.Info("Performing finalizer operations before delete CR ..")
			// 1. set status "Downgrade"
			meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
				Type:    fmt.Sprint("Unavailable"),
				Status:  metav1.ConditionFalse,
				Reason:  "MyApphasbeenmarkedDeleted",
				Message: fmt.Sprintf("performing finalizer operations for MyApp obj: %s update status False", myapp.Name),
			})
			// update status
			if err := r.Update(ctx, myapp); err != nil {
				log.Error(err, "performing finalizer operations update status False with marked delete....")
				return ctrl.Result{}, err
			}
			// perform all operation before remove finalizer
			if err := r.doFinalizerOperationForMyApp(ctx, myapp); err != nil {
				log.Error(err, "Failed to perform operation before remove finalizer")
				return ctrl.Result{}, err
			}

			// fetch the latest MyApp obj
			if err := r.Get(ctx, req.NamespacedName, myapp); err != nil {
				log.Error(err, "Failed re-fetch MyApp obj after finalizer operation")
				return ctrl.Result{}, err
			}

			// set status after finalizer operation
			meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
				Type:    fmt.Sprint("Unavailable"),
				Status:  metav1.ConditionTrue,
				Reason:  "AfterFinalizerOperation",
				Message: fmt.Sprintf("Finalizer operation for custom resource MyApp: %s was successfully", myapp.Name),
			})
			if err := r.Status().Update(ctx, myapp); err != nil {
				log.Error(err, "Failed to update MyApp obj status after finalizer operation")
				return ctrl.Result{}, err
			}
			// everything is ok,now remove finalizer
			log.Info("Removing Finalizer for MyApp obj after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(myapp, MyAppFinalizer); !ok {
				log.Error(err, "Failed to remove Finalizer for MyApp")
				return ctrl.Result{Requeue: true}, nil // toDO ctrl.Result{}, err 区别
			}
			if err = r.Update(ctx, myapp); err != nil {
				log.Error(err, "Failed to update MyApp obj After remove finalizer")
				return ctrl.Result{}, err
			}
		}
		// no finalizer
		return ctrl.Result{}, nil
	}

	// check if the deployment already exists, if not create new one
	deploy := &appsv1.Deployment{}
	err = r.Get(ctx, req.NamespacedName, deploy)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForMyApp(myapp)
		if err != nil {
			log.Error(err, "Failed to define a new Deployment for MyApp")
			// update status
			meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
				Type:    fmt.Sprint("Available"),
				Status:  metav1.ConditionFalse,
				Reason:  "FailedtoDefineAnewDeployForMyApp",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", myapp.Name, err),
			})
			if err := r.Status().Update(ctx, myapp); err != nil {
				log.Error(err, "Failed to update MyApp status with failed define Deployment")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully
		// todo: We will requeue the reconciliation so that we can ensure the state and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil

	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}
	// deployment already exists, check if spec.size update
	// oldsize
	size := myapp.Spec.Size
	if *deploy.Spec.Replicas != size {
		log.Info("MyApp.deployment.spec.size changed,update size")
		deploy.Spec.Replicas = &size
		// updateSize
		if err := r.Update(ctx, deploy); err != nil {
			log.Error(err, "Failed to update Deployment Size", "Deployment.Name", deploy.Name, "Deployment.Namespace", deploy.Namespace)
			// todo
			// Re-fetch the memcached Custom Resource before updating the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raising the error "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, deploy); err != nil {
				log.Error(err, "Failed to re-fetch Deployment")
				return ctrl.Result{}, err
			}
			// re-fetch deployment Failed, set conditions
			meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
				Type:    fmt.Sprint("Available"),
				Reason:  fmt.Sprint("Resizing"),
				Status:  metav1.ConditionFalse,
				Message: fmt.Sprintf("Failed to update size for Deployment (%s):(%s)", myapp.Name, err),
			})
			if err := r.Update(ctx, myapp); err != nil {
				log.Error(err, "Failed to update size for deployment")
				return ctrl.Result{}, err
			}
		}
		// todo  ???
		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}
	// finally update status
	meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
		Type:    fmt.Sprint("Available"),
		Status:  metav1.ConditionTrue,
		Reason:  "ReSize",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", myapp.Name, size)})

	if err := r.Status().Update(ctx, myapp); err != nil {
		log.Error(err, "Failed to update MyApp status")
		return ctrl.Result{}, err
	}

	// check if the service already exits, if not create new one
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: myapp.Name + "-svc", Namespace: myapp.Namespace}, service)
	if err != nil && apierrors.IsNotFound(err) {
		svc, err := r.serviceForMyApp(myapp)
		if err != nil {
			log.Error(err, "Failed to define a new Service for MyApp")
			meta.SetStatusCondition(&myapp.Status.Conditions, metav1.Condition{
				Type:    fmt.Sprint("Available"),
				Status:  metav1.ConditionFalse,
				Reason:  "FailedToDefineAnewSvcForMyApp",
				Message: fmt.Sprintf("Failed to create Service for the custom resource (%s): (%s)", myapp.Name, err),
			})
			if err := r.Status().Update(ctx, myapp); err != nil {
				log.Error(err, "Failed to update MyApp status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new Service", "Serivce.Namespace", svc.Namespace, "Service.Name", svc.Name)
		if err := r.Create(ctx, svc); err != nil {
			log.Error(err, "Failed to create a new Service", "Service.Namespace", svc.Namespace, "Serivce.Name", svc.Name)
			return ctrl.Result{}, err
		}
		// Service created successfully
		// todo: We will requeue the reconciliation so that we can ensure the state and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// perform operation before remove finalizer
func (r *MyAppReconciler) doFinalizerOperationForMyApp(ctx context.Context, myapp *echohv1alpha1.MyApp) error {
	fmt.Printf("Check if the Deployment: %s, Service: %s in Namespace: %s, has been Deleted?\n", myapp.Name, myapp.Name+"svc", myapp.Namespace)
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: myapp.Namespace, Name: myapp.Name}, dep); err == nil {
		if err := r.Delete(ctx, dep); err != nil {
			return err
		}
	}
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: myapp.Namespace, Name: myapp.Name + "svc"}, svc); err == nil {
		if err := r.Delete(ctx, dep); err != nil {
			return err
		}
	}
	return nil
}

// get ports
func getPorts(myapp *echohv1alpha1.MyApp) (svcPort, nodePort int32, cotainerPort intstr.IntOrString) {
	for _, ports := range myapp.Spec.Ports {
		cotainerPort = ports.TargetPort
		svcPort = ports.Port
		nodePort = ports.NodePort
	}
	return svcPort, nodePort, cotainerPort
}

// define a new deployment
func (r *MyAppReconciler) deploymentForMyApp(myapp *echohv1alpha1.MyApp) (*appsv1.Deployment, error) {
	_, containerPort, _ := getPorts(myapp)
	ls := labelsForMyApp(myapp)
	replicas := myapp.Spec.Size
	image := myapp.Spec.Image
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      myapp.Name,
			Namespace: myapp.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      myapp.Name,
					Namespace: myapp.Namespace,
					Labels:    ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            myapp.Name + "container",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{ContainerPort: containerPort},
							},
						},
					},
				},
			},
		},
	}
	// set ownerRef
	if err := ctrl.SetControllerReference(myapp, dep, r.Scheme); err != nil {
		fmt.Printf("set OwnerRef for Deployment Failed .....")
		return nil, err
	}
	return dep, nil

}

// define a new service
func (r *MyAppReconciler) serviceForMyApp(myapp *echohv1alpha1.MyApp) (*corev1.Service, error) {
	svcPort, nodePort, containerPort := getPorts(myapp)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      myapp.Name + "-svc",
			Namespace: myapp.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labelsForMyApp(myapp),
			Ports: []corev1.ServicePort{
				{
					Port:       svcPort,
					NodePort:   nodePort,
					TargetPort: containerPort,
				},
			},
		},
	}
	// set ownerRef
	if err := ctrl.SetControllerReference(myapp, svc, r.Scheme); err != nil {
		fmt.Printf("set OwnerRef for Service Failed .....")
		return nil, err
	}
	return svc, nil
}

func labelsForMyApp(myapp *echohv1alpha1.MyApp) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "MyApp-operator",
		"app.kubernetes.io/managed-by": "MyAppController",
		"app":                          myapp.Name,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&echohv1alpha1.MyApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
