package reconcilers // Adjust to match your package name

import (
	"context"
	"testing"

	"github.com/openshift-pipelines/pipelines-kueue-addon/internal/manifests"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// We assume saName is defined as a constant or variable in your package
// const saName = "multikueue-sa"

func TestEnsureSpokeRBAC(t *testing.T) {
	// 1. Temporarily override the embedded YAML variable with a simple, predictable manifest
	originalYAML := manifest.SpokeManifest
	defer func() { manifest.SpokeManifest = originalYAML }()

	manifest.SpokeManifest = []byte(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: test-cluster-role
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
`)

	namespace := "spoke-ns"
	expectedClusterRoleName := "test-cluster-role"
	expectedBindingName := saName + "-clusterbinding"

	tests := []struct {
		name          string
		existingObjs  []*corev1.ServiceAccount // Pre-existing objects for update testing
		existingRoles []*rbacv1.ClusterRole
		existingBinds []*rbacv1.ClusterRoleBinding
		expectError   bool
	}{
		{
			name: "Fresh creation - no resources exist",
			// empty clientset
		},
		{
			name: "Update existing - resources already exist with old data",
			existingObjs: []*corev1.ServiceAccount{
				{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace}},
			},
			existingRoles: []*rbacv1.ClusterRole{
				{
					ObjectMeta: metav1.ObjectMeta{Name: expectedClusterRoleName},
					// Old rules that should get overwritten by our mocked YAML
					Rules: []rbacv1.PolicyRule{{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create"}}},
				},
			},
			existingBinds: []*rbacv1.ClusterRoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: expectedBindingName},
					// Old subjects that should get overwritten
					Subjects: []rbacv1.Subject{{Kind: "User", Name: "old-user"}},
					RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "old-role", APIGroup: "rbac.authorization.k8s.io"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize the fake client with any pre-existing objects
			fakeClient := fake.NewSimpleClientset()
			for _, obj := range tt.existingObjs {
				fakeClient.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
			}
			for _, obj := range tt.existingRoles {
				fakeClient.RbacV1().ClusterRoles().Create(context.TODO(), obj, metav1.CreateOptions{})
			}
			for _, obj := range tt.existingBinds {
				fakeClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), obj, metav1.CreateOptions{})
			}

			r := &MultiKueueReconciler{}
			err := r.ensureSpokeRBAC(context.TODO(), fakeClient, namespace)

			if (err != nil) != tt.expectError {
				t.Fatalf("expected error: %v, got: %v", tt.expectError, err)
			}

			// Validate ServiceAccount creation
			_, err = fakeClient.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), saName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("expected ServiceAccount %q to exist", saName)
			}

			// Validate ClusterRole creation/update
			cr, err := fakeClient.RbacV1().ClusterRoles().Get(context.TODO(), expectedClusterRoleName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("expected ClusterRole %q to exist", expectedClusterRoleName)
			}
			if len(cr.Rules) != 1 || cr.Rules[0].Resources[0] != "pods" {
				t.Errorf("ClusterRole rules did not match expected YAML. Got: %+v", cr.Rules)
			}

			// Validate ClusterRoleBinding creation/update
			crb, err := fakeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), expectedBindingName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("expected ClusterRoleBinding %q to exist", expectedBindingName)
			}
			if len(crb.Subjects) != 1 || crb.Subjects[0].Name != saName || crb.Subjects[0].Namespace != namespace {
				t.Errorf("ClusterRoleBinding subjects are incorrect. Got: %+v", crb.Subjects)
			}
			if crb.RoleRef.Name != expectedClusterRoleName {
				t.Errorf("ClusterRoleBinding roleRef is incorrect. Expected %q, got: %q", expectedClusterRoleName, crb.RoleRef.Name)
			}
		})
	}
}
