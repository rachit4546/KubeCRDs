package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	webappv1 "github.com/rachit4645/KubeCRDs/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Globals used by tests
var (
	k8sManager ctrl.Manager
)

var _ = Describe("CRDs Controller", func() {
	const timeout = time.Second * 10
	const interval = time.Millisecond * 250

	var (
		appKey types.NamespacedName
		app    *webappv1.CRDs
	)

	BeforeEach(func() {
		// Initialize appKey and app for each test
		appKey = types.NamespacedName{
			Name:      "test-app",
			Namespace: "default",
		}
		replicas := int32(2)
		app = &webappv1.CRDs{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appKey.Name,
				Namespace: appKey.Namespace,
			},
			Spec: webappv1.AppSpec{
				Image:    "nginx:latest",
				Replicas: &replicas,
			},
		}
	})

	AfterEach(func() {
		if app != nil {
			_ = k8sClient.Delete(ctx, app)
			// Wait for deletion to complete
			Eventually(func() bool {
				err := k8sClient.Get(ctx, appKey, app)
				return err != nil && errors.IsNotFound(err)
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		}
	})

	It("should create a Deployment when App is created", func() {
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, appKey, deployment)
		}, timeout, interval).Should(Succeed())

		Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:latest"))
	})

	It("should update Deployment replicas when App.spec.replicas changes", func() {
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		Eventually(func() error {
			d := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, appKey, d); err != nil {
				return err
			}
			replicas := int32(3)
			d.Spec.Replicas = &replicas
			return k8sClient.Update(ctx, d)
		}, timeout, interval).Should(Succeed())
	})

	It("should delete Deployment when App is deleted", func() {
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		Expect(k8sClient.Delete(ctx, app)).To(Succeed())

		deployment := &appsv1.Deployment{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, appKey, deployment)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})
})
