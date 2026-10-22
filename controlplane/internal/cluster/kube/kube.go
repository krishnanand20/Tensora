// Package kube is the real cluster.Backend: client-go with server-side apply.
package kube

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/domain"
)

type kindInfo struct {
	resource   string
	namespaced bool
}

// allowed is every kind the controller may create. Apply refuses anything else, so a bug in
// a builder can't create, say, a ClusterRoleBinding. Keep in sync with deploy/charts/tensora RBAC.
var allowed = map[schema.GroupVersionKind]kindInfo{
	{Version: "v1", Kind: "Namespace"}:                                         {"namespaces", false},
	{Version: "v1", Kind: "Service"}:                                           {"services", true},
	{Version: "v1", Kind: "ResourceQuota"}:                                     {"resourcequotas", true},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"}:                        {"statefulsets", true},
	{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}:         {"networkpolicies", true},
	{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"}:           {"ciliumnetworkpolicies", true},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"}:   {"rolebindings", true},
	{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}:              {"virtualmachines", true},
	{Group: "serving.kserve.io", Version: "v1beta1", Kind: "InferenceService"}: {"inferenceservices", true},
}

var (
	vmGVR   = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
	stsGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	isvcGVR = schema.GroupVersionResource{Group: "serving.kserve.io", Version: "v1beta1", Resource: "inferenceservices"}
	svcGVR  = schema.GroupVersionResource{Version: "v1", Resource: "services"}
	pvcGVR  = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}

	// cleanup is what Delete removes, primary objects first.
	cleanup = []schema.GroupVersionResource{vmGVR, isvcGVR, stsGVR, svcGVR, pvcGVR}
)

type Backend struct {
	dyn     dynamic.Interface
	core    kubernetes.Interface
	catalog *catalog.Catalog
}

// New uses in-cluster config, falling back to KUBECONFIG / ~/.kube/config.
func New(cat *catalog.Catalog) (*Backend, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("kubernetes config: %w", err)
		}
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Backend{dyn: dyn, core: core, catalog: cat}, nil
}

func (b *Backend) Apply(ctx context.Context, objs []cluster.Manifest) error {
	for _, m := range objs {
		u := &unstructured.Unstructured{Object: m}
		gvk := u.GroupVersionKind()
		info, ok := allowed[gvk]
		if !ok {
			return fmt.Errorf("refusing to apply %s: kind not in allowlist", gvk)
		}
		res := b.dyn.Resource(gvk.GroupVersion().WithResource(info.resource))
		var ri dynamic.ResourceInterface = res
		if info.namespaced {
			ri = res.Namespace(u.GetNamespace())
		}
		_, err := ri.Apply(ctx, u.GetName(), u, metav1.ApplyOptions{FieldManager: cluster.FieldManager, Force: true})
		if err != nil {
			return fmt.Errorf("apply %s %s/%s: %w", gvk.Kind, u.GetNamespace(), u.GetName(), err)
		}
	}
	return nil
}

func (b *Backend) Observe(ctx context.Context, ns string, id uuid.UUID, kind domain.WorkloadKind) (cluster.Observed, string, error) {
	var gvr schema.GroupVersionResource
	var judge func(*unstructured.Unstructured) (cluster.Observed, string)
	switch kind {
	case domain.KindVM:
		gvr, judge = vmGVR, observeVM
	case domain.KindNotebook:
		gvr, judge = stsGVR, observeStatefulSet
	case domain.KindEndpoint:
		gvr, judge = isvcGVR, observeInferenceService
	default:
		return "", "", fmt.Errorf("unknown workload kind %q", kind)
	}
	u, err := b.dyn.Resource(gvr).Namespace(ns).Get(ctx, cluster.ObjectName(id), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return cluster.ObservedAbsent, "", nil
	}
	if err != nil {
		return "", "", err
	}
	if u.GetDeletionTimestamp() != nil {
		return cluster.ObservedProgressing, "deleting", nil
	}
	o, reason := judge(u)
	return o, reason, nil
}

func (b *Backend) Delete(ctx context.Context, ns string, id uuid.UUID) error {
	sel := metav1.ListOptions{LabelSelector: cluster.LabelWorkloadID + "=" + id.String()}
	background := metav1.DeletePropagationBackground
	for _, gvr := range cleanup {
		ri := b.dyn.Resource(gvr).Namespace(ns)
		list, err := ri.List(ctx, sel)
		if apierrors.IsNotFound(err) {
			continue // CRD not installed
		}
		if err != nil {
			return fmt.Errorf("list %s: %w", gvr.Resource, err)
		}
		for _, item := range list.Items {
			err := ri.Delete(ctx, item.GetName(), metav1.DeleteOptions{PropagationPolicy: &background})
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete %s %s/%s: %w", gvr.Resource, ns, item.GetName(), err)
			}
		}
	}
	return nil
}

func (b *Backend) Capacity(ctx context.Context) (map[string]int, error) {
	nodes, err := b.core.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	out := map[string]int{}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		if n.Spec.Unschedulable || !nodeReady(n) {
			continue
		}
		for _, gc := range b.catalog.GPUClasses {
			out[gc.ID] += nodeCapacity(n, gc)
		}
	}
	return out, nil
}

// ---- pure helpers (unit-tested)

func nodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// nodeCapacity counts a node's GPUs of one class. Passthrough GPUs are identified by their
// device resource; container GPUs share nvidia.com/gpu, so the node selector tells classes apart.
func nodeCapacity(n *corev1.Node, gc catalog.GPUClass) int {
	total := 0
	if gc.VMDeviceName != "" {
		total += allocatable(n, gc.VMDeviceName)
	}
	if len(gc.NodeSelector) > 0 && labelsMatch(n.Labels, gc.NodeSelector) {
		total += allocatable(n, gc.ContainerResource)
	}
	return total
}

func allocatable(n *corev1.Node, resource string) int {
	q, ok := n.Status.Allocatable[corev1.ResourceName(resource)]
	if !ok {
		return 0
	}
	return int(q.Value())
}

func labelsMatch(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func observeVM(u *unstructured.Unstructured) (cluster.Observed, string) {
	status, _, _ := unstructured.NestedString(u.Object, "status", "printableStatus")
	switch status {
	case "Running":
		return cluster.ObservedReady, status
	case "Stopped":
		return cluster.ObservedStopped, status
	}
	// Errors such as ErrorUnschedulable may clear on their own; the controller's
	// provisioning timeout decides when to give up.
	return cluster.ObservedProgressing, status
}

func observeStatefulSet(u *unstructured.Unstructured) (cluster.Observed, string) {
	want, found, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if !found {
		want = 1
	}
	ready, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	switch {
	case want == 0 && ready == 0:
		return cluster.ObservedStopped, "scaled to zero"
	case want > 0 && ready >= want:
		return cluster.ObservedReady, ""
	}
	return cluster.ObservedProgressing, fmt.Sprintf("%d/%d replicas ready", ready, want)
}

func observeInferenceService(u *unstructured.Unstructured) (cluster.Observed, string) {
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conditions {
		cond, _ := c.(map[string]any)
		if cond["type"] != "Ready" {
			continue
		}
		if cond["status"] == "True" {
			return cluster.ObservedReady, ""
		}
		msg, _ := cond["message"].(string)
		return cluster.ObservedProgressing, msg
	}
	return cluster.ObservedProgressing, "waiting for status"
}
